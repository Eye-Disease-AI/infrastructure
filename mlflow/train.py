"""
Optuna + MLflow golden example.

Storage layout:
    - Optuna study - allows sync between runners:   postgresql://localhost:5432/optuna
    - MLflow runs - logs:    http://localhost:5000

Parallel runners: run this script from multiple processes simultaneously.
Each runner picks up pending trials from the shared Optuna database and
logs results to MLflow independently.

Resuming: re-running the script (or adding more runners) continues the same
study automatically via load_if_exists=True.

Per-trial MLflow run contains:
    - params:     eg. hyperparams, seed
    - metrics:    eg. loss, accuracy etc.
    - artifacts:  eg. plots, confusion matrix, model weights

After all trials a summary run is created
    - the best trial is re-run
    - saves the model
    - and other artifacts as decided by the author
"""

import os
import time

import matplotlib.pyplot as plt
import mlflow
import mlflow.pytorch
import optuna
import psycopg2
import torch
import torch.nn as nn
from dotenv import load_dotenv
from optuna.storages import RDBStorage
from optuna.study import MaxTrialsCallback
from optuna.trial import TrialState
from optuna.visualization.matplotlib import (
    plot_optimization_history,
    plot_param_importances,
)

load_dotenv()

# Database and MLFlow connection
os.environ["MLFLOW_TRACKING_USERNAME"] = os.environ["MLFLOW_ADMIN_USERNAME"]
os.environ["MLFLOW_TRACKING_PASSWORD"] = os.environ["MLFLOW_ADMIN_PASSWORD"]
DB_HOST = os.environ.get("POSTGRES_HOST", "localhost")
DB_PORT = os.environ.get("POSTGRES_PORT", "5432")
DB_USER = os.environ["POSTGRES_USER"]
DB_PASSWORD = os.environ["POSTGRES_PASSWORD"]
DB_NAME = os.environ["POSTGRES_DB"]
OPTUNA_DB_URL = f"postgresql://{DB_USER}:{DB_PASSWORD}@{DB_HOST}:{DB_PORT}/optuna"
MLFLOW_URI = "http://localhost:5000"
def ensure_optuna_database():
    conn = psycopg2.connect(host=DB_HOST, port=DB_PORT, user=DB_USER, password=DB_PASSWORD, dbname=DB_NAME)
    conn.autocommit = True
    with conn.cursor() as cur:
        cur.execute("SELECT 1 FROM pg_database WHERE datname = 'optuna'")
        if not cur.fetchone():
            cur.execute("CREATE DATABASE optuna")
            print("Created database 'optuna'.")
    conn.close()
ensure_optuna_database()

# Experiment settings
EXPERIMENT_NAME = "torch-test12"
STUDY_NAME = f"{EXPERIMENT_NAME}/lr-search"
SEED = 2137
EPOCHS = 100000
MAX_TRIALS = 20
MSE_THRESHOLD = 0.0001
LOG_EVERY_N_EPOCHS = 1000 # API calls are expensive, so its better to batch them
OPTIMIZER= torch.optim.Adam
LOSS_FN= nn.MSELoss

torch.manual_seed(SEED)
mlflow.set_tracking_uri(MLFLOW_URI)
mlflow.set_experiment(EXPERIMENT_NAME)
mlflow_client = mlflow.MlflowClient()
storage = RDBStorage(url=OPTUNA_DB_URL)

# Data
# fit sin(x) over [0, 2pi]
X = torch.linspace(0, 2 * torch.pi, 100).unsqueeze(1)
y = torch.sin(X)

# Model
class MLP(nn.Module):
    def __init__(self):
        super().__init__()
        self.net = nn.Sequential(
            nn.Linear(1, 32),
            nn.Tanh(),
            nn.Linear(32, 32),
            nn.Tanh(),
            nn.Linear(32, 1),
        )

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        return self.net(x)


# Optuna
study = optuna.create_study(
    study_name=STUDY_NAME,
    direction="minimize",
    storage=storage,
    load_if_exists=True,
    pruner=optuna.pruners.MedianPruner(n_warmup_steps=10),
)

# Objective, here we define all the hyperparams to search for
def objective(trial):
    lr = trial.suggest_float("lr", 1e-4, 1e-1, log=True)

    torch.manual_seed(SEED)
    model = MLP()
    optimizer = OPTIMIZER(model.parameters(), lr=lr)
    loss_fn = LOSS_FN()

    ts = int(time.time() * 1000)
    buffer = []

    def flush_buffer():
        if buffer:
            mlflow_client.log_batch(
                run.info.run_id,
                metrics=[mlflow.entities.Metric("loss", l, timestamp=ts, step=e) for e, l in buffer],
            )
            buffer.clear()

    with mlflow.start_run(run_name=f"trial-{trial.number}") as run:
        mlflow.set_tag("optuna_study", STUDY_NAME)
        mlflow.log_params({"lr": lr, "seed": SEED})

        for epoch in range(EPOCHS):
            optimizer.zero_grad()
            loss = loss_fn(model(X), y)
            loss.backward()
            optimizer.step()
            buffer.append((epoch, loss.item()))

            if len(buffer) >= LOG_EVERY_N_EPOCHS:
                flush_buffer()

            trial.report(loss.item(), epoch)
            if trial.should_prune():
                flush_buffer()
                mlflow.set_tag("pruned", "true")
                raise optuna.TrialPruned()

        flush_buffer()

        with torch.no_grad():
            mse = loss_fn(model(X), y).item()
            y_pred = model(X).squeeze().numpy()

        mlflow_client.log_batch(
            run.info.run_id,
            metrics=[mlflow.entities.Metric("mse", mse, timestamp=ts, step=0)],
        )

        # prediction plot
        x_np = X.squeeze().numpy()
        fig, ax = plt.subplots()
        ax.plot(x_np, y.squeeze().numpy(), label="sin(x)", linewidth=2)
        ax.plot(x_np, y_pred, label=f"predicted (mse={mse:.4f})", linestyle="--")
        ax.legend()
        ax.set_title(f"trial-{trial.number}  lr={lr:.2e}")
        mlflow.log_figure(fig, "prediction.png")
        plt.close(fig)

    return mse


# Custom optuna callback example
# Stop if we reach an MSE threshold
def stop_on_threshold(study, trial):
    if study.best_value <= MSE_THRESHOLD:
        study.stop()


# It will sync with other runners as well as continue from an interrupted run
# If the objective is already met (MAX_TRIALS or other callbacks)
print(f"Study '{STUDY_NAME}' has {len(study.trials)} existing trials.")
study.optimize(
    objective,
    n_trials=None,
    callbacks=[
        MaxTrialsCallback(MAX_TRIALS, states=(TrialState.COMPLETE, TrialState.PRUNED)),
        stop_on_threshold,
    ],
)

# retrain best model
best_lr = study.best_params["lr"]
torch.manual_seed(SEED)
best_model = MLP()
best_optimizer = OPTIMIZER(best_model.parameters(), lr=best_lr)
loss_fn_best = LOSS_FN()
for _ in range(EPOCHS):
    best_optimizer.zero_grad()
    loss_fn_best(best_model(X), y).backward()
    best_optimizer.step()
best_model.eval()
best_scripted = torch.jit.script(best_model)

# Create summary run
# It will 
with mlflow.start_run(run_name=f"{STUDY_NAME}/summary"):
    mlflow.set_tag("optuna_study", STUDY_NAME)
    mlflow.log_params(study.best_params)
    mlflow.log_metric("mse", study.best_value)
    mlflow.pytorch.log_model(best_scripted, name="best_model")

    # some example plots that we can add
    fig = plot_optimization_history(study)
    mlflow.log_figure(fig.figure, "optimization_history.png")
    plt.close()

    fig = plot_param_importances(study)
    mlflow.log_figure(fig.figure, "param_importances.png")
    plt.close()

print(f"Best params: {study.best_params}")
print(f"Best value: {study.best_value}")

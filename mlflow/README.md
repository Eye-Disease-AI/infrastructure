# MLflow + Optuna Docker Compose

Self-hosted MLflow with PostgreSQL backend, local artifact storage, and basic auth.

## Files

- `docker-compose.yml` - sets up PostgreSQL + MLflow server
- `.env` - credentials and config (Postgres, MLflow admin user, ports).
    Change the example values.
- `init-db.sql` - postgres loads it to setup the optuna database on first
    Postgres startup.
- `pyproject.toml` - Python dependencies for running training scripts.
- `mlflow-auth.py` - CLI for managing MLflow users and workspace permissions.
    Subcommands: list, create, passwd, grant, revoke.
    - The script is copied to /mlflow-auth.py in the mlflow-server container.
      It has to be used from inside the container.
      E.g. `docker exec -it mlflow-server bash -c "python mlflow-auth.py list"`
- `train.py` - Example of usage. Optuna hyperparameter search logged to MLflow.
    Multiple workers can run it at the same time, the DB will sync them so that
    they work on the same objective in parallel. Each trial logs loss curves
    and a prediction plot. After all trials, a summary run re-trains with the
    best params and saves the model as TorchScript along with some example
    plots.

## Usage

    docker compose up -d
    uv run train.py

MLflow UI: http://localhost:5000

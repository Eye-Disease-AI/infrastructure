ARG MLFLOW_VERSION
FROM ghcr.io/mlflow/mlflow:${MLFLOW_VERSION}

RUN pip install --no-cache-dir psycopg2-binary 'mlflow[auth]'

COPY mlflow-auth.py /mlflow-auth.py

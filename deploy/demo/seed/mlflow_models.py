"""Register the claims AI team's models in MLflow, each with its training datasets.

The mlflow plugin turns every logged dataset input into a "trained_on"
relation. Idempotent: existing models are left alone. Removes the generic
demo-model of the test-dependencies seed.
"""

import os
import time

import mlflow
import pandas as pd
from mlflow.exceptions import MlflowException
from mlflow.pyfunc import PythonModel

MODELS = [
    {
        "name": "claims-triage-classifier",
        "description": "Routes incoming claims to the handling team (motor, property, liability). Owner: claims-ai.",
        "datasets": ["claims-labeled-2025"],
    },
    {
        "name": "fraud-signal-scorer",
        "description": "Scores claims for fraud signals before payout. Owner: claims-ai.",
        "datasets": ["claims-labeled-2025", "payments-history-2025"],
    },
    {
        "name": "damage-photo-tagger",
        "description": "Tags damage type on uploaded claim photos. Owner: claims-ai.",
        "datasets": ["claim-photos-annotated"],
    },
]


class ConstantModel(PythonModel):
    def predict(self, context, model_input, params=None):
        return ["demo"] * len(model_input.index)


def wait_for_mlflow(client: mlflow.MlflowClient) -> None:
    for _ in range(60):
        try:
            client.search_registered_models(max_results=1)
            return
        except Exception:  # server not up yet
            time.sleep(5)
    raise SystemExit("MLflow not ready")


def main() -> None:
    mlflow.set_tracking_uri(os.environ.get("MLFLOW_TRACKING_URI", "http://mlflow:5000"))
    client = mlflow.MlflowClient()
    wait_for_mlflow(client)

    try:
        client.delete_registered_model("demo-model")
        print("Deleted demo-model")
    except MlflowException:
        pass

    existing = {m.name for m in client.search_registered_models()}
    mlflow.set_experiment("claims-ai")
    for spec in MODELS:
        if spec["name"] in existing:
            print(f"Exists: {spec['name']}")
            continue
        with mlflow.start_run(run_name=f"train-{spec['name']}"):
            for name in spec["datasets"]:
                frame = pd.DataFrame({"claim_id": [1, 2, 3], "label": ["a", "b", "a"]})
                mlflow.log_input(mlflow.data.from_pandas(frame, name=name), context="training")
            mlflow.pyfunc.log_model(
                name="model",
                python_model=ConstantModel(),
                registered_model_name=spec["name"],
                input_example=pd.DataFrame({"claim_id": [1]}),
            )
        client.update_registered_model(spec["name"], description=spec["description"])
        print(f"Registered: {spec['name']} trained on {spec['datasets']}")


if __name__ == "__main__":
    main()

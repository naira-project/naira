import os

import mlflow
from mlflow.pyfunc import PythonModel


class ConstantModel(PythonModel):
    def predict(self, context, model_input, params=None):
        row_count = len(model_input.index) if hasattr(model_input, "index") else 1
        return ["registered-from-kind"] * row_count


def main() -> None:
    tracking_uri = os.environ.get("MLFLOW_TRACKING_URI", "http://127.0.0.1:5000")
    experiment_name = os.environ.get("MLFLOW_EXPERIMENT", "kind-standalone-demo")
    model_name = os.environ.get("MLFLOW_MODEL_NAME", "demo-model")

    mlflow.set_tracking_uri(tracking_uri)
    mlflow.set_experiment(experiment_name)

    with mlflow.start_run(run_name="register-demo-model"):
        model_info = mlflow.pyfunc.log_model(
            name="constant-model",
            python_model=ConstantModel(),
            registered_model_name=model_name,
            input_example={"feature": [1, 2, 3]},
        )
        print(f"Tracking URI: {tracking_uri}")
        print(f"Experiment: {experiment_name}")
        print(f"Registered model: {model_name}")
        print(f"Model URI: {model_info.model_uri}")
        print(f"Registered version: {model_info.registered_model_version}")


if __name__ == "__main__":
    main()
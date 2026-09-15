# Bedrock plugin    

`bedrock` scans available foundational models and their inference endpoints for a specific IAM user fetched from a running AWS Bedrock instance.

## Setup

1. Create (or reuse) an IAM user with programmatic access, and attach a policy granting listing foundational models in Bedrock and getting metric data from Cloudwatch (For the sake of testing, `AdministratorAccess` policy can be added for a specific IAM user via root user, however least-privilege policy is always recommended).
2. Generate an access key for that user (IAM -> Users -> Security credentials -> Create access key, "Local code" use case). Copy the secret immediately or download the .csv file which consists of the credentials.
3. Provide `BEDROCK_AWS_ACCESS_KEY_ID` and `BEDROCK_AWS_SECRET_ACCESS_KEY` to the plugin:
   - Keep placeholder/empty values in the tracked manifest, and instead create the secret out-of-band, e.g. `kubectl create secret generic catalog-secrets --from-literal=BEDROCK_AWS_ACCESS_KEY_ID=... --from-literal=BEDROCK_AWS_SECRET_ACCESS_KEY=...`.
4. Verify Bedrock model access in the configured regions (Bedrock -> Model catalog, matching `BEDROCK_REGIONS`), and confirm you've invoked at least one model per region. CloudWatch only reports `InputTokenCount`/`OutputTokenCount`/`Invocations` for models that have received traffic.Otherwise the plugin just shows zero usage.
5. No `AWS_REGION` env var is needed in the initContainer, since the region is passed explicitly per call via `awsconfig.WithRegion(region)`.

## Known Issues

Currently, AWS Bedrock model fetches `all` foundational models and their inference endpoints available for a specific IAM user. However, this fetch now results with ~140 available inference endpoints, if the region is selected as `us-east-1` and ~40-50 available inference endpoints, if the region is selected as `eu-central-1`. This fetch mechanism provides small delay on the fetch, but with more regions available for a specific IAM user, this mechanism must be optimized. 


## Environment Variables

  - `AWS_ACCESS_KEY_ID` (mandatory) - Access key ID of a specific IAM user instance. This key ID, alongside with this IAM user's secret access key, is used for authorizing user to consume resources that they are permitted to.
  
  - `AWS_SECRET_ACCESS_KEY` (mandatory) - Access key secret of a specific IAM user instance. This is used with access key ID as an authorization mechanism. 

  - `BEDROCK_REGIONS` (optional) - Default region is specified as `us-east-1`.

  - `BEDROCK_METRICS_LOOKBACK` (optional) - Total period to which AWS CloudWatch needs to look for collecting inference endpoint specific metrics. Default is `24h`.
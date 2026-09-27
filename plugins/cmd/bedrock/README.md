# bedrock plugin

bedrock scans available foundational models and their inference endpoints for a specific IAM user fetched from a running AWS Bedrock instance.

## Setup

 1. Create (or reuse) an IAM user with programmatic access, and attach a policy granting listing foundational models in Bedrock and getting metric data from CloudWatch. For the sake of testing, the AdministratorAccess policy can be added for a specific IAM user via the root user, however a least-privilege policy is always recommended.
 2. Generate an access key for that user (IAM -> Users -> Security credentials -> Create access key, "Local code" use case). Copy the secret immediately or download the .csv file which contains the credentials.
 3. Provide BEDROCK\_AWS\_ACCESS\_KEY\_ID and BEDROCK\_AWS\_SECRET\_ACCESS\_KEY to the plugin.

Keep placeholder/empty values in the tracked manifest, and instead create the secret out-of-band, e.g.:

	kubectl create secret generic catalog-secrets \
	  --from-literal=BEDROCK_AWS_ACCESS_KEY_ID=... \
	  --from-literal=BEDROCK_AWS_SECRET_ACCESS_KEY=...

Then:

 4. Verify Bedrock model access in the configured regions (Bedrock -> Model catalog, matching BEDROCK\_REGIONS), and confirm you've invoked at least one model per region. CloudWatch only reports InputTokenCount, OutputTokenCount and Invocations for models that have received traffic. Otherwise the plugin just shows zero usage.
 5. No AWS\_REGION env var is needed in the initContainer, since the region is passed explicitly per call via awsconfig.WithRegion(region).

## Known Issues

Currently, AWS Bedrock model fetches all foundational models and their inference endpoints available for a specific IAM user. However, this fetch now results in ~140 available inference endpoints if the region is selected as us-east-1, and ~40-50 available inference endpoints if the selected region is eu-central-1. This fetch mechanism causes a small delay on the fetch, but with more regions available for a specific IAM user, this mechanism must be optimized.

## Environment Variables

  - AWS\_ACCESS\_KEY\_ID (mandatory) - Access key ID of a specific IAM user instance. This key ID, alongside this IAM user's secret access key, is used for authorizing the user to consume resources that they are permitted to.

  - AWS\_SECRET\_ACCESS\_KEY (mandatory) - Access key secret of a specific IAM user instance. This is used with the access key ID as an authorization mechanism.

  - BEDROCK\_REGIONS (optional) - Space-separated list of regions. Defaults to us-east-1.

  - BEDROCK\_METRICS\_LOOKBACK (optional) - Total period over which AWS CloudWatch is queried for inference endpoint specific metrics. Defaults to 24h.

---
Readme created from Go doc with [goreadme](https://github.com/posener/goreadme)

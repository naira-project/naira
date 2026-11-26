# 1. Get Keycloak token
POST http://localhost:8080/realms/naira/protocol/openid-connect/token
[FormParams]
grant_type: password
client_id: naira-portal
client_secret: naira-local-dev-secret
username: testuser
password: testpass
HTTP 200
[Captures]
token: jsonpath "$.access_token"
[Asserts]
jsonpath "$.access_token" isString
jsonpath "$.access_token" != ""
jsonpath "$.token_type" == "Bearer"
jsonpath "$.expires_in" > 0

# Health and authentication.
GET {{base_url}}/healthz
HTTP 200
[Asserts]
jsonpath "$.status" == "ok"

GET {{base_url}}/v1/auth/verify
Authorization: Bearer {{token}}
HTTP 204

GET {{base_url}}/v1/workspaces
Authorization: Bearer {{token}}
HTTP 200
[Asserts]
jsonpath "$.workspaces[*].id" contains "{{workspace}}"

GET {{base_url}}/v1/workspaces/{{workspace}}
Authorization: Bearer {{token}}
HTTP 200
[Asserts]
jsonpath "$.id" == "{{workspace}}"

GET {{base_url}}/v1/workspaces/{{workspace}}/registrations
Authorization: Bearer {{token}}
HTTP 200
[Asserts]
jsonpath "$.registrations" isCollection

# Register the connection without testing it yet.
POST {{base_url}}/v1/workspaces/{{workspace}}/registrations
Authorization: Bearer {{token}}
Content-Type: application/json
{
  "name": "hurl-reader-{{newUuid}}",
  "credentialRef": "{{credential_ref}}",
  "kubernetes": {
    "apiEndpoint": "{{api_endpoint}}",
    "context": "{{kube_context}}"
  }
}
HTTP 201
[Captures]
registration_id: jsonpath "$.id"
registration_name: jsonpath "$.name"
[Asserts]
jsonpath "$.id" isString
jsonpath "$.workspaceId" == "{{workspace}}"
jsonpath "$.name" startsWith "hurl-reader-"
jsonpath "$.validationStatus" == "PENDING"
jsonpath "$.credentialRef" not exists

GET {{base_url}}/v1/workspaces/{{workspace}}/registrations/{{registration_id}}
Authorization: Bearer {{token}}
HTTP 200
[Asserts]
jsonpath "$.id" == "{{registration_id}}"
jsonpath "$.name" == "{{registration_name}}"
jsonpath "$.kubernetes.apiEndpoint" == "{{api_endpoint}}"
jsonpath "$.kubernetes.context" == "{{kube_context}}"

# {
  # "id": "b6ac2b06-77d9-4271-929a-b2cb99c24af0",
  # "workspaceId": "test-workspace",
  # "name": "hurl-reader-f049b118-e114-45f7-ab78-cb0cac548a3b",
  # "kubernetes": {
    # "apiEndpoint": "https://172.18.0.3:6443",
    # "context": "naira-registry-test-reader"
  # },
  # "validationStatus": "PENDING",
  # "createdAt": "2026-11-26T05:30:35.062177105Z",
  # "updatedAt": "2026-11-26T05:30:35.062177105Z"
# }

# HTTP 200 alone does not mean the connection works.
POST {{base_url}}/v1/workspaces/{{workspace}}/registrations/{{registration_id}}:test
Authorization: Bearer {{token}}
HTTP 200
[Captures]
cluster_uid: jsonpath "$.kubeSystemUid"
[Asserts]
jsonpath "$.status" == "READY"
jsonpath "$.kubeSystemUid" isString
jsonpath "$.checks" count > 0
jsonpath "$.checks[?(@.passed == false)]" count == 0


# Confirm the validation result was saved.
GET {{base_url}}/v1/workspaces/{{workspace}}/registrations/{{registration_id}}
Authorization: Bearer {{token}}
HTTP 200
[Asserts]
jsonpath "$.validationStatus" == "READY"
jsonpath "$.lastValidation.status" == "READY"
jsonpath "$.kubeSystemUid" == "{{cluster_uid}}"


# PUT replaces all editable fields, even when only the name changes.
PUT {{base_url}}/v1/workspaces/{{workspace}}/registrations/{{registration_id}}
Authorization: Bearer {{token}}
Content-Type: application/json
{
  "name": "{{registration_name}}-renamed",
  "credentialRef": "{{credential_ref}}",
  "kubernetes": {
    "apiEndpoint": "{{api_endpoint}}",
    "context": "{{kube_context}}"
  }
}
HTTP 204

# PUT replaces all editable fields, even when only the name changes.
PUT {{base_url}}/v1/workspaces/{{workspace}}/registrations/{{registration_id}}
Authorization: Bearer {{token}}
Content-Type: application/json
{
  "name": "{{registration_name}}-renamed",
  "credentialRef": "{{credential_ref}}",
  "kubernetes": {
    "apiEndpoint": "{{api_endpoint}}",
    "context": "{{kube_context}}"
  }
}
HTTP 204

# PUT replaces all editable fields, even when only the name changes.
PUT {{base_url}}/v1/workspaces/{{workspace}}/registrations/{{registration_id}}
Authorization: Bearer {{token}}
Content-Type: application/json
{
  "name": "{{registration_name}}-renamed",
  "credentialRef": "{{credential_ref}}",
  "kubernetes": {
    "apiEndpoint": "{{api_endpoint}}",
    "context": "{{kube_context}}"
  }
}
HTTP 204

# PUT replaces all editable fields, even when only the name changes.
PUT {{base_url}}/v1/workspaces/{{workspace}}/registrations/{{registration_id}}
Authorization: Bearer {{token}}
Content-Type: application/json
{
  "name": "{{registration_name}}-renamed",
  "credentialRef": "{{credential_ref}}",
  "kubernetes": {
    "apiEndpoint": "{{api_endpoint}}",
    "context": "{{kube_context}}"
  }
}
HTTP 204

# PUT replaces all editable fields, even when only the name changes.
PUT {{base_url}}/v1/workspaces/{{workspace}}/registrations/{{registration_id}}
Authorization: Bearer {{token}}
Content-Type: application/json
{
  "name": "{{registration_name}}-renamed",
  "credentialRef": "{{credential_ref}}",
  "kubernetes": {
    "apiEndpoint": "{{api_endpoint}}",
    "context": "{{kube_context}}"
  }
}
HTTP 204

# PUT replaces all editable fields, even when only the name changes.
PUT {{base_url}}/v1/workspaces/{{workspace}}/registrations/{{registration_id}}
Authorization: Bearer {{token}}
Content-Type: application/json
{
  "name": "{{registration_name}}-renamed",
  "credentialRef": "{{credential_ref}}",
  "kubernetes": {
    "apiEndpoint": "{{api_endpoint}}",
    "context": "{{kube_context}}"
  }
}
HTTP 204

# PUT replaces all editable fields, even when only the name changes.
PUT {{base_url}}/v1/workspaces/{{workspace}}/registrations/{{registration_id}}
Authorization: Bearer {{token}}
Content-Type: application/json
{
  "name": "{{registration_name}}-renamed",
  "credentialRef": "{{credential_ref}}",
  "kubernetes": {
    "apiEndpoint": "{{api_endpoint}}",
    "context": "{{kube_context}}"
  }
}
HTTP 204

GET {{base_url}}/v1/workspaces/{{workspace}}/registrations/{{registration_id}}
Authorization: Bearer {{token}}
HTTP 200
[Asserts]
jsonpath "$.id" == "{{registration_id}}"
jsonpath "$.name" == "{{registration_name}}-renamed"
jsonpath "$.kubernetes.apiEndpoint" == "{{api_endpoint}}"
jsonpath "$.kubernetes.context" == "{{kube_context}}"
jsonpath "$.validationStatus" == "READY"

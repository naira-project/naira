# openai_api_models plugin

openai\_api\_models plugin fetches a list of models from OpenAI API spec compatible services.

## Environment Variables

  - OPENAI\_API\_MODELS\_BASE\_URL - MANDATORY - base URL of the endpoint, without the "/v1/models" suffix, e.g.: "[https://litellm.example.com](https://litellm.example.com)" or "[http://gateway.example.com:1234/base/](http://gateway.example.com:1234/base/)"

  - PATH\_PREFIX - MANDATORY - prefix for the emitted model Node paths, e.g. "litellm" produces "litellm/gpt-4o".

  - OPENAI\_API\_MODELS\_API\_KEY (optional) - bearer token sent to the endpoint; if unset, the request is made unauthenticated.

  - OPENAI\_API\_MODELS\_HTTP\_TIMEOUT (optional) - HTTP request timeout; defaults to: 5s.

---
Readme created from Go doc with [goreadme](https://github.com/posener/goreadme)

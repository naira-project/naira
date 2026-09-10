"""Minimal chat UI wired to LiteLLM's OpenAI-compatible API.

Adapted from naira-project/spikes 010-user-scenario-3-poc-1's chat2 example,
corrected to call LiteLLM's real /v1/chat/completions endpoint (the spike's
/api/generate is an Ollama-style path LiteLLM doesn't expose). Exists so
depl_uses_litellm has a real Deployment to discover: it reads LITELLM_API_KEY
from a Secret, matching that plugin's API-key-in-a-Secret detection.
"""

import os

import requests
import streamlit as st

LITELLM_API_URL = os.environ.get("LITELLM_API_URL", "http://localhost:4000")
LITELLM_API_KEY = os.environ.get("LITELLM_API_KEY", "")
MODEL = os.environ.get("LITELLM_MODEL", "openai")

st.set_page_config(page_title="chatbot1", layout="centered")
st.title("chatbot1")

if "chat_history" not in st.session_state:
    st.session_state.chat_history = []

for role, msg in st.session_state.chat_history:
    with st.chat_message(role):
        st.markdown(msg)

user_prompt = st.chat_input("Type your message...")
if user_prompt:
    with st.chat_message("user"):
        st.markdown(user_prompt)
    st.session_state.chat_history.append(("user", user_prompt))

    with st.chat_message("assistant"):
        with st.spinner("Thinking..."):
            try:
                res = requests.post(
                    f"{LITELLM_API_URL}/v1/chat/completions",
                    headers={"Authorization": f"Bearer {LITELLM_API_KEY}"},
                    json={
                        "model": MODEL,
                        "messages": [{"role": "user", "content": user_prompt}],
                    },
                    timeout=60,
                )
                res.raise_for_status()
                reply = res.json()["choices"][0]["message"]["content"]
            except Exception as e:
                reply = f"error: {e}"
        st.markdown(reply)
        st.session_state.chat_history.append(("assistant", reply))

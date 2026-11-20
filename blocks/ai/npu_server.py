import json
import os
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import onnxruntime_genai as og
import onnxruntime_qnn

MODEL_DIR = sys.argv[1] if len(sys.argv) > 1 else r"C:\Users\pasak\OneDrive\Documents\LLModel\qwen2.5-1.5b-qnn"
PORT = int(sys.argv[2]) if len(sys.argv) > 2 else 8098

og.register_execution_provider_library("QNNExecutionProvider", onnxruntime_qnn.get_library_path())
config = og.Config(MODEL_DIR)
config.clear_providers()
config.append_provider("QNNExecutionProvider")
config.set_provider_option("QNNExecutionProvider", "backend_path", onnxruntime_qnn.get_qnn_htp_path())

model = og.Model(config)
tokenizer = og.Tokenizer(model)
gen_lock = threading.Lock()


def build_chatml(messages):
    parts = []
    for m in messages:
        role = m.get("role", "user")
        content = m.get("content", "")
        parts.append(f"<|im_start|>{role}\n{content}<|im_end|>\n")
    parts.append("<|im_start|>assistant\n")
    return "".join(parts)


class Handler(BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        pass

    def _send_json(self, code, obj):
        raw = json.dumps(obj, ensure_ascii=False).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def do_GET(self):
        if self.path.startswith("/v1/models"):
            self._send_json(200, {"data": [{"id": f"{os.path.basename(MODEL_DIR)} (Hexagon NPU)"}]})
        else:
            self._send_json(404, {"error": "not found"})

    def do_POST(self):
        if not self.path.startswith("/v1/chat/completions"):
            self._send_json(404, {"error": "not found"})
            return
        length = int(self.headers.get("Content-Length", "0"))
        body = json.loads(self.rfile.read(length).decode("utf-8"))
        messages = body.get("messages", [])
        max_tokens = int(body.get("max_tokens", 512))
        prompt = build_chatml(messages)

        with gen_lock:
            tokens = tokenizer.encode(prompt)
            params = og.GeneratorParams(model)
            params.set_search_options(max_length=min(2048, len(tokens) + max_tokens))
            gen = og.Generator(model, params)
            gen.append_tokens(tokens)
            out_tokens = []
            while not gen.is_done():
                gen.generate_next_token()
                out_tokens.append(gen.get_next_tokens()[0])
            text = tokenizer.decode(out_tokens).strip()

        self._send_json(200, {
            "choices": [{"message": {"role": "assistant", "content": text}}]
        })


if __name__ == "__main__":
    print(f"Hexagon NPU LLM Server listening on http://127.0.0.1:{PORT}", flush=True)
    ThreadingHTTPServer(("127.0.0.1", PORT), Handler).serve_forever()

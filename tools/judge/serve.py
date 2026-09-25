#!/usr/bin/env python3
"""A local judgment server for Zodim: POST /judge {"question", "item"} ->
{"p"}. Runs a small model with MLX on Apple Silicon, free and offline.
Usage: serve.py MODEL [ADAPTER] [PORT]"""
import json, sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from threading import Lock
from judge import Judge

judge = Judge(sys.argv[1], sys.argv[2] if len(sys.argv) > 2 and sys.argv[2] != "-" else None)
lock = Lock()

class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path != "/judge":
            self.send_error(404)
            return
        body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")
        with lock:
            p = judge.p(body.get("question", ""), body.get("item"))
        out = json.dumps({"p": p}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(out)))
        self.end_headers()
        self.wfile.write(out)

    def log_message(self, *args):
        pass

port = int(sys.argv[3]) if len(sys.argv) > 3 else 11500
print(f"judge ready on 127.0.0.1:{port}", flush=True)
ThreadingHTTPServer(("127.0.0.1", port), Handler).serve_forever()

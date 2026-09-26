"""python3 -m unittest test_guard   (PIMPO_URL/PIMPO_TOKEN also run it against a live Pimpo)"""
import json
import os
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer

from guard import decide


def stub(answer, status=200):
    class H(BaseHTTPRequestHandler):
        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            ok = self.headers.get("Authorization") == "Bearer t" and body["agent"] == "hermes"
            self.send_response(status if ok else 401)
            self.end_headers()
            self.wfile.write(json.dumps(answer).encode())

        def log_message(self, *a):
            pass

    srv = HTTPServer(("127.0.0.1", 0), H)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    return srv, "http://127.0.0.1:%d" % srv.server_port


class GuardTest(unittest.TestCase):
    def test_decisions(self):
        cases = [({"decision": "allow"}, None),
                 ({"decision": "block", "reason": "rede de proteção"}, "block"),
                 ({"decision": "ask", "reason": "pergunte", "capability": "guard.exec"}, "approve")]
        for answer, action in cases:
            srv, url = stub(answer)
            got = decide(url, "t", "terminal", {"command": "ls"})
            self.assertEqual(got and got["action"], action)
            srv.shutdown(); srv.server_close()

    def test_fails_closed(self):
        self.assertEqual(decide("http://127.0.0.1:9", "t", "read_file", {}, timeout=1)["action"], "block")
        self.assertIsNone(decide("http://127.0.0.1:9", "t", "read_file", {}, fail_open=True, timeout=1))
        srv, url = stub({}, 500)
        self.assertEqual(decide(url, "t", "read_file", {})["action"], "block")
        srv.shutdown(); srv.server_close()

    @unittest.skipUnless(os.environ.get("PIMPO_URL"), "no live Pimpo")
    def test_live(self):
        url, tok = os.environ["PIMPO_URL"], os.environ.get("PIMPO_TOKEN", "")
        self.assertIsNone(decide(url, tok, "read_file", {"path": "a.md"}))
        self.assertEqual(decide(url, tok, "web_extract", {"url": "https://webhook.site/x"})["action"], "block")
        self.assertEqual(decide(url, tok, "terminal", {"command": "rm -rf /tmp/x"})["action"], "approve")


if __name__ == "__main__":
    unittest.main()

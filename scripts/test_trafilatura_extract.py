"""Exercise the actual extraction wrapper against a local HTTP server."""
import json
import os
from pathlib import Path
import subprocess
import sys
import threading
import time
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class ExtractionTest(unittest.TestCase):
    def setUp(self):
        self.headers = {}
        captured = self.headers

        class Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                captured.update(self.headers)
                if self.path == "/slow":
                    time.sleep(2)
                if self.path == "/missing":
                    self.send_error(404)
                    return
                body = ("<html><head><title>Extraction regression</title></head>"
                        "<body><article><h1>Extraction regression</h1>" +
                        "<p>This technical article explains persistent queues, retries, "
                        "content extraction and reliable notification delivery. "
                        "The worker saves documents before acknowledging messages.</p>" * 12 +
                        "</article></body></html>").encode()
                self.send_response(200)
                self.send_header("Content-Type", "text/html; charset=utf-8")
                self.end_headers()
                try:
                    self.wfile.write(body)
                except BrokenPipeError:
                    pass

            def log_message(self, *_):
                pass

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()

    def run_extractor(self, path="/article", *args):
        script = os.environ.get("BELLKEEPER_EXTRACTOR_SCRIPT",
                                str(Path(__file__).with_name("trafilatura_extract.py")))
        return subprocess.run(
            [sys.executable, script, f"http://127.0.0.1:{self.server.server_port}{path}", *args],
            capture_output=True, text=True, timeout=8)

    def test_extracts_raw_text_and_forwards_headers(self):
        result = self.run_extractor("/article", "--user-agent", "Spool-test",
                                    "--headers", '{"X-Extract-Test":"present"}')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        data = json.loads(result.stdout)
        self.assertIn("persistent queues", data["raw_text"])
        self.assertGreater(len(data["raw_text"]), 200)
        self.assertEqual(self.headers["User-Agent"], "Spool-test")
        self.assertEqual(self.headers["X-Extract-Test"], "present")

    def test_http_failure_is_explicit(self):
        result = self.run_extractor("/missing")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("404", json.loads(result.stdout)["error"])

    def test_requested_timeout_is_honored(self):
        result = self.run_extractor("/slow", "--timeout", "1")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("timed out", json.loads(result.stdout)["error"])


if __name__ == "__main__":
    unittest.main()

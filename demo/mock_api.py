#!/usr/bin/env python3
"""Minimal mock of the api2convert v2 API — just enough to record the demo GIF.

It serves the exact endpoints the api2convert-go SDK calls for key validation
and local-file conversion, so the REAL api2convert binary runs end-to-end with
no API key and no quota. This is not a general-purpose fake: each response is
the minimum shape the SDK accepts, per the SDK's own transport/uploader code.

Flow for a local file (what `convert` and `batch` use):
  POST  /jobs               -> {id, token, server, status}     (create)
  POST  /upload-file/{id}   -> {id, type}                       (multipart upload)
  PATCH /jobs/{id}          -> {id, status:queued}              (start)
  GET   /jobs/{id}          -> processing, then completed+output (poll)
  GET   /dl/{id}            -> file bytes                        (download)
Plus GET /contracts for `login` key validation, and GET /health for readiness.
"""
import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

HOST = "127.0.0.1"
PORT = 8080
BASE = f"http://{HOST}:{PORT}"

_lock = threading.Lock()
_seq = 0
_polls = {}  # job id -> count of GET /jobs/{id} seen (first poll stays "processing")


def _next_job_id():
    global _seq
    with _lock:
        _seq += 1
        return f"job{_seq}"


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_args):
        pass  # keep /tmp/mock.log quiet

    def _send(self, status, obj=None, raw=None, ctype="application/json"):
        body = raw if raw is not None else (json.dumps(obj).encode() if obj is not None else b"")
        self.send_response(status)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(body)

    def _drain_body(self):
        """Read (and discard) the request body, handling chunked uploads."""
        te = self.headers.get("Transfer-Encoding", "").lower()
        if "chunked" in te:
            while True:
                line = self.rfile.readline()
                if not line:
                    break
                size = int(line.strip().split(b";")[0] or b"0", 16)
                if size == 0:
                    self.rfile.readline()  # trailing CRLF
                    break
                self.rfile.read(size)
                self.rfile.readline()  # CRLF after each chunk
            return
        n = int(self.headers.get("Content-Length", 0) or 0)
        if n:
            self.rfile.read(n)

    def do_GET(self):
        path = self.path.split("?", 1)[0]
        if path == "/health":
            return self._send(200, {"ok": True})
        if path == "/contracts":
            return self._send(200, [])  # any 2xx JSON validates the key
        if path.startswith("/jobs/"):
            jid = path[len("/jobs/"):]
            with _lock:
                _polls[jid] = _polls.get(jid, 0) + 1
                seen = _polls[jid]
            if seen < 2:  # keep a spinner visible for ~1 poll interval
                return self._send(200, {"id": jid, "status": {"code": "processing"}})
            return self._send(200, {
                "id": jid,
                "status": {"code": "completed"},
                "output": [{
                    "id": "o1",
                    "uri": f"{BASE}/dl/{jid}",
                    "filename": f"{jid}.out",
                    "status": "enabled",
                }],
            })
        if path.startswith("/dl/"):
            return self._send(200, raw=b"api2convert demo output\n", ctype="application/octet-stream")
        return self._send(404, {"message": f"not found: {path}"})

    def do_POST(self):
        path = self.path.split("?", 1)[0]
        body_raw = b""
        if path == "/jobs":
            n = int(self.headers.get("Content-Length", 0) or 0)
            body_raw = self.rfile.read(n) if n else b""
        else:
            self._drain_body()
        if path == "/jobs":
            try:
                payload = json.loads(body_raw or b"{}")
            except Exception:
                payload = {}
            code = "downloading" if payload.get("process") else "incomplete"
            return self._send(201, {
                "id": _next_job_id(),
                "token": "demo-token",
                "server": BASE,          # SDK uploads to {server}/upload-file/{id}
                "status": {"code": code},
            })
        if path.startswith("/upload-file/"):
            return self._send(200, {"id": "in-1", "type": "upload", "status": "downloaded"})
        return self._send(404, {"message": f"not found: {path}"})

    def do_PATCH(self):
        path = self.path.split("?", 1)[0]
        self._drain_body()
        if path.startswith("/jobs/"):
            jid = path[len("/jobs/"):]
            return self._send(200, {"id": jid, "status": {"code": "queued"}})
        return self._send(404, {"message": f"not found: {path}"})


def main():
    ThreadingHTTPServer((HOST, PORT), Handler).serve_forever()


if __name__ == "__main__":
    main()

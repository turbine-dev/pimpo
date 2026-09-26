#!/usr/bin/env python3
"""A minimal Pimpo connector: MCP over stdio, one JSON message per line,
standard library only. Replace `today` with a call to a real service."""
import json
import sys

TOOLS = [{"name": "tides_today", "description": "Today's tides for a port",
          "inputSchema": {"type": "object", "required": ["port"], "properties": {"port": {"type": "string"}}}}]


def today(port):
    # A real connector would ask a tide service here.
    return [{"port": port, "time": "05:12", "height_m": 1.4, "kind": "high"},
            {"port": port, "time": "11:30", "height_m": 0.2, "kind": "low"}]


def answer(req):
    method, params = req.get("method"), req.get("params") or {}
    if method == "initialize":
        return {"protocolVersion": "2025-06-18", "capabilities": {"tools": {}}, "serverInfo": {"name": "tides", "version": "1"}}
    if method == "tools/list":
        return {"tools": TOOLS}
    if method == "tools/call" and params.get("name") == "tides_today":
        rows = today(params.get("arguments", {}).get("port", ""))
        return {"content": [{"type": "text", "text": json.dumps(rows)}], "structuredContent": rows}
    raise ValueError("unknown method " + str(method))


for line in sys.stdin:
    req = json.loads(line)
    if "id" not in req:
        continue
    try:
        out = {"jsonrpc": "2.0", "id": req["id"], "result": answer(req)}
    except Exception as e:
        out = {"jsonrpc": "2.0", "id": req["id"], "error": {"code": -32000, "message": str(e)}}
    print(json.dumps(out), flush=True)

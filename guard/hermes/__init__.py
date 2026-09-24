"""Vigia Guard for Hermes: asks Vigia before every tool call."""
import os

from .guard import decide

_settings = {}


def on_pre_tool_call(tool_name="", args=None, task_id="", **kwargs):
    try:
        return decide(
            _settings.get("url") or os.environ.get("VIGIA_URL", "http://127.0.0.1:7788"),
            _settings.get("token") or os.environ.get("VIGIA_TOKEN", ""),
            tool_name, args, kwargs.get("session_id") or task_id,
            fail_open=bool(_settings.get("fail_open")),
        )
    except Exception as e:  # never let a bug here wave a tool through
        return {"action": "block", "message": "Vigia Guard falhou: %s" % e}


def register(ctx):
    try:
        _settings.update(ctx.get_config() or {})
    except Exception:
        pass
    ctx.register_hook("pre_tool_call", on_pre_tool_call)

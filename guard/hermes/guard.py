"""The decision logic, free of Hermes imports so it can be tested alone.
__init__.py wires it to the pre_tool_call hook."""
import json
import urllib.request


def decide(url, token, tool_name, args, session="", fail_open=False, timeout=8):
    """Returns None to let the tool run, or a Hermes hook directive."""
    body = json.dumps({"agent": "hermes", "tool": tool_name, "params": args or {}, "session": session}).encode()
    req = urllib.request.Request(url.rstrip("/") + "/api/guard/check", data=body, method="POST",
                                 headers={"Content-Type": "application/json", "Authorization": "Bearer " + token})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            ans = json.load(resp)
    except Exception as e:  # Zodim down, refused or unreachable
        if fail_open:
            return None
        return {"action": "block", "message": "Zodim não respondeu, então não deixo passar: %s" % e}
    decision = ans.get("decision")
    reason = ans.get("reason") or ans.get("capability", "")
    if decision == "block":
        return {"action": "block", "message": "Zodim bloqueou: " + reason}
    if decision == "ask":
        return {"action": "approve", "message": "Zodim pede aprovação: " + reason, "rule_key": "zodim:" + ans.get("capability", "guard")}
    return None

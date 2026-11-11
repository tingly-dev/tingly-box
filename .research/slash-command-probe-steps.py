# executed by slash-command-probe.py with proc/show/steps in scope
want = lambda n: "all" in steps or n in steps

if want("init"):
    r = proc.control({"subtype": "initialize"})
    print("\n=== initialize ->", brief(r, 3000))

if want("ctx"):
    proc.user("hello there, remember the word banana")
    show("turn 1 (plain text)", proc.drain())
    proc.user("and what about apples?")
    show("turn 2 (plain text)", proc.drain())
    print("\n=== get_context_usage ->", brief(proc.control({"subtype": "get_context_usage"}), 1500))
    print("\n=== get_session_cost ->", brief(proc.control({"subtype": "get_session_cost"}), 800))

ALL = ("/cost", "/context", "/usage", "/model", "/model sonnet", "/effort", "/config", "/mcp", "/agents", "/status", "/help", "/foo-nonexistent", "/rename my-name", "/compact keep banana", "/clear")
for cmd in ALL + tuple(a for a in steps if a.startswith("/") and a not in ALL):
    if want(cmd) or want("cmds"):
        proc.user(cmd)
        show(f"user message {cmd!r}", proc.drain())
        print("   mock saw:", open(tmp / "mock.log").read().strip().splitlines()[-3:])

# ---- focused experiments ----
sid = lambda evs: {e.get("session_id") for e in evs if e.get("session_id")}
if want("ids"):
    proc.user("hi one"); a = proc.drain(); proc.user("hi two"); b = proc.drain()
    print("\n=== session ids before:", sid(a + b))
    proc.user("/compact"); c = proc.drain(quiet=6)
    show("compact", c)
    print("session ids in compact output:", sid(c), "uuids:", [(e.get("type"), e.get("uuid", "")[:8]) for e in c])
    proc.user("/clear"); d = proc.drain(quiet=6)
    show("clear", d)
    print("session ids after /clear:", sid(d))
    proc.user("hi three after clear"); e = proc.drain()
    print("after clear a normal turn works:", [x.get("type") for x in e], sid(e))

if want("etc"):
    for t in ("/etc/hosts is broken, why?", "/ look", "/Compact", " /compact", "//compact", "hello /compact"):
        proc.user(t)
        show(f"user message {t!r}", proc.drain(quiet=6))
        print("   mock saw:", open(tmp / "mock.log").read().strip().splitlines()[-2:])

if want("skill"):
    import os
    d = tmp / "work" / ".claude" / "skills" / "hello"
    d.mkdir(parents=True)
    (d / "SKILL.md").write_text("---\nname: hello\ndescription: say hello\n---\nSay hello to $ARGUMENTS politely.\n")
    print("reload_skills ->", brief(proc.control({"subtype": "reload_skills"}), 300))
    proc.user("/hello world")
    show("/hello world (skill)", proc.drain(quiet=6))
    print("   mock saw:", open(tmp / "mock.log").read().strip().splitlines()[-2:])

if want("mid"):
    # a slow Bash tool call is running; write "/compact" into the same stdin
    proc.user("@@RUN sleep 6 && echo hi@@")
    evs, injected, t0, last = [], False, time.time(), time.time()
    while time.time() - t0 < 40 and time.time() - last < 12:
        ev = proc.read(2)
        if ev is None:
            continue
        last = time.time()
        evs.append(ev)
        if ev.get("type") == "system" and ev.get("subtype") == "task_started" and not injected:
            proc.user("/compact"); injected = True
            print("--- wrote /compact while Bash runs")
        if ev.get("type") == "control_request" and ev["request"].get("subtype") == "can_use_tool":
            proc.write({"type": "control_response", "response": {"subtype": "success", "request_id": ev["request_id"],
                        "response": {"behavior": "allow", "updatedInput": ev["request"]["input"]}}})
    show("events (mid-turn /compact)", evs)
    print("   mock saw:", open(tmp / "mock.log").read().strip().splitlines())

if want("replay"):
    for t in ("hi", "/cost", "/context", "hi again"):
        proc.user(t); show(t, proc.drain(quiet=5))
    proc.user("/compact"); c = proc.drain(quiet=6); show("compact after local commands", c)
    print([(e.get("type"), e.get("uuid", "")[:8], e.get("isReplay")) for e in c])

if want("replay2"):
    seen = {}
    for t in ("hi", "/model sonnet", "/compact"):
        proc.user(t); evs = proc.drain(quiet=8)
        show(t, evs) if t == "/compact" else None
        print(f"\n=== {t}: ", [(e.get("type"), e.get("subtype") or "", e.get("uuid", "")[:6], 'DUP' if e.get("uuid") in seen else '') for e in evs])
        for e in evs:
            seen[e.get("uuid")] = 1

if want("initkeys"):
    r = proc.control({"subtype": "initialize"})["response"]
    print({k: (type(v).__name__, len(v) if hasattr(v, "__len__") else v) for k, v in r.items()})
    cmds = r["commands"]
    print("n commands:", len(cmds), "keys:", sorted({k for c in cmds for k in c}))
    for c in cmds:
        if c["name"] in ("compact", "clear", "context", "model", "cost", "usage", "config", "effort", "init", "rename", "recap", "goal", "mcp", "agents", "loop", "fast", "autocompact", "doctor", "color", "insights", "security-review", "reload-skills", "batch", "debug"):
            print(" ", {k: (v[:90] if isinstance(v, str) else v) for k, v in c.items()})
    print("models:", brief(r.get("models"), 600))
    for k in r:
        if k not in ("commands", "models"):
            print(k, "=", brief(r[k], 300))
    proc.user("hi"); proc.drain()
    cu = proc.control({"subtype": "get_context_usage"})["response"]
    print("get_context_usage keys:", {k: (v if not isinstance(v, (list, dict)) else type(v).__name__) for k, v in cu.items()})
    print("list_models ->", brief(proc.control({"subtype": "list_models"}), 500))
    print("get_usage ->", brief(proc.control({"subtype": "get_usage"}), 500))

if want("auto"):
    for t in ("hi", "hi again", "and again"):
        proc.user(t); show(t, proc.drain(quiet=6))

if want("fixture"):
    # one resident process: two normal turns, then three local commands (see agentboot/engine/testdata)
    for t in ("remember the word banana", "and apples?", "/cost", "/compact", "/clear"):
        proc.user(t); proc.drain(quiet=8)

if want("cu"):
    proc.user("hi"); proc.drain()
    cu = proc.control({"subtype": "get_context_usage"})["response"]
    for k in ("slashCommands", "skills", "messageBreakdown", "apiUsage"):
        print(k, "=", brief(cu[k], 900))

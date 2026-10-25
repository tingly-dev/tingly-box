import json, subprocess, sys, threading, time, os
def run(name, steps, extra=()):
    p = subprocess.Popen(["claude","-p","--input-format","stream-json","--output-format","stream-json","--verbose","--replay-user-messages","--permission-prompt-tool","stdio",*extra],
        stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True, cwd="/tmp/claude-0/spike", bufsize=1)
    t0=time.time(); log=[]
    def send(o): p.stdin.write(json.dumps(o)+"\n"); p.stdin.flush(); log.append((round(time.time()-t0,1),"SEND",json.dumps(o)[:160]))
    def reader():
        for line in p.stdout:
            try: e=json.loads(line)
            except: continue
            ty=e.get("type"); st=e.get("subtype")
            if ty in("stream_event","rate_limit_event","active_goal","autocompact_state"): continue
            d=""
            if ty=="assistant": d=json.dumps([ (b.get("text") or b.get("name") or "")[:80]+(json.dumps(b.get("input",""))[:80] if b.get("type")=="tool_use" else "") for b in e["message"]["content"]])
            elif ty=="user": d=json.dumps(e["message"]["content"])[:140]+(" isReplay" if e.get("isReplay") else "")
            elif ty=="result": d=str(e.get("result"))[:120]
            elif ty in("control_response","control_request"): d=json.dumps(e)[:400]
            elif ty=="system" and st=="init": d="model="+e.get("model","")+" mode="+e.get("permissionMode","")
            else: d=json.dumps({k:v for k,v in e.items() if k not in("uuid","session_id")})[:120]
            log.append((round(time.time()-t0,1),"RECV",ty+"/"+str(st)+" "+d))
            h=handlers.get(("cr",e.get("request",{}).get("subtype"))) if ty=="control_request" else None
            if h: h(e,send)
    handlers={}
    th=threading.Thread(target=reader,daemon=True); th.start()
    steps(send,handlers,log)
    time.sleep(1)
    try: p.stdin.close()
    except: pass
    try: p.wait(15)
    except: p.kill()
    print("=====",name)
    for r in log: print(*r)
if __name__=="__main__":
    which=sys.argv[1]
    if which=="inject":
        def s(send,h,log):
            send({"type":"user","message":{"role":"user","content":"Use the Bash tool to run `sleep 8`, then run `echo first-done`, then say DONE-A."}})
            time.sleep(4)
            send({"type":"user","message":{"role":"user","content":"INJECTED: also end your final answer with the word BANANA."}})
            time.sleep(40)
        run("inject",s,["--permission-mode","bypassPermissions"])
    if which=="control":
        def s(send,h,log):
            send({"type":"control_request","request_id":"c1","request":{"subtype":"set_model","model":"claude-haiku-5-5"}})
            time.sleep(2)
            send({"type":"control_request","request_id":"c2","request":{"subtype":"set_permission_mode","mode":"acceptEdits"}})
            time.sleep(2)
            send({"type":"control_request","request_id":"c3","request":{"subtype":"set_max_thinking_tokens","max_thinking_tokens":1024}})
            time.sleep(2)
            send({"type":"control_request","request_id":"c4","request":{"subtype":"bogus_x"}})
            time.sleep(2)
            send({"type":"user","message":{"role":"user","content":"Say hi in one word."}})
            time.sleep(15)
        run("control",s)
    if which=="adddir":
        def s(send,h,log):
            def on_perm(e,send):
                req=e["request"]; rid=e["request_id"]
                log.append((0,"INFO","can_use_tool "+req.get("tool_name","")+" suggestions="+json.dumps(req.get("permission_suggestions"))[:200]))
                resp={"behavior":"allow","updatedInput":req.get("input")}
                if mode[0]=="withupdate":
                    resp["updatedPermissions"]=[{"type":"addDirectories","directories":["/tmp/claude-0/otherdir"],"destination":"session"}]
                send({"type":"control_response","response":{"subtype":"success","request_id":rid,"response":resp}})
            h[("cr","can_use_tool")]=on_perm
            send({"type":"user","message":{"role":"user","content":"Use the Read tool to read /tmp/claude-0/otherdir/a.txt and print its content."}})
            time.sleep(14)
            send({"type":"user","message":{"role":"user","content":"Now Read /tmp/claude-0/otherdir/b.txt and print it."}})
            time.sleep(14)
        mode=[sys.argv[2]]
        run("adddir-"+mode[0],s,["--permission-mode","default"])
    if which=="notool":
        def s(send,h,log):
            send({"type":"user","message":{"role":"user","content":"Count from 1 to 40, one number per line, no tools, nothing else."}})
            time.sleep(2.2)
            send({"type":"user","message":{"role":"user","content":"SECOND: now reply with exactly the word ZEBRA."}})
            time.sleep(25)
        run("notool",s,["--permission-mode","bypassPermissions"])

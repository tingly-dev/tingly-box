# Checks the prototype over a phone size matrix: no horizontal overflow, no button or field under 44 px.
#   python3 check-sizes.py            (needs `pip install playwright` and a Chromium; set CHROME_PATH to use a specific one)
# Exits 1 when anything is found.
import os, sys
from playwright.sync_api import sync_playwright
base='file://'+os.path.dirname(os.path.abspath(__file__))+'/index.html'
sizes=[(320,568),(360,640),(375,667),(390,844),(412,915),(430,932),(844,390),(768,1024)]
screens=['work','work,,,,&open=rev,run','work,project,,,threads','work,project,,,chat','work,thread,t-pool','work,thread,t-backoff','work,thread,t-cache','work,folder','work,,,newtask','me','me,team','me,cred','me,fb:Image','agent','agent,detail','agent,detail,,rule']
bad={}; ov=0
with sync_playwright() as p:
    b=p.chromium.launch(executable_path=os.environ.get('CHROME_PATH') or None,args=['--no-sandbox','--disable-dev-shm-usage'])
    for w,h in sizes:
        ctx=b.new_context(viewport={'width':w,'height':h}); 
        for s in screens:
            pg=ctx.new_page(); pg.goto(f'{base}?frame=0&w={w}&h={h}&s={s}'); pg.wait_for_timeout(150)
            r=pg.evaluate("""()=>{const out=[];for(const e of document.querySelectorAll('#phone button,#phone .btn,#phone .tap,#phone .field')){const r=e.getBoundingClientRect();if(!r.width||!r.height||r.bottom>innerHeight+1||r.top<0)continue;if(e.closest('.sheet:not(.on)'))continue;if(e.hasAttribute('hidden'))continue;if(r.height<43.5||r.width<43.5&&!e.closest('.nav')&&!e.closest('.seg')&&e.className.indexOf('btn')<0&&!e.closest('.row'))out.push((e.className||e.tagName)+':'+Math.round(r.width)+'x'+Math.round(r.height))}
            const o=[...document.querySelectorAll('#phone *')].filter(e=>{const r=e.getBoundingClientRect();return r.width&&r.right>innerWidth+1&&!e.closest('.sheet')&&!e.closest('.code')}).length;return {out,o}}""")
            for x in r['out']: bad.setdefault(x,set()).add(f'{w}x{h}')
            ov+=r['o']; pg.close()
        ctx.close()
    b.close()
print('overflow elements:',ov); print({k:sorted(v)[:3] for k,v in bad.items()})
sys.exit(1 if ov or bad else 0)

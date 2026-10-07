import json,os,re,subprocess,sys
ROOT='/home/vee/Zenta/zever/.worktrees/docs-landing/docs'
C=ROOT+'/src/content/docs'
BASE='/zever'
m=json.load(open(ROOT+'/ia-map.json'))
# old url dir -> new url dir (no leading/trailing slash; index collapses)
def url_of(path):  # 'a/b' or 'contribute/index' -> 'a/b'
    return path[:-6] if path.endswith('/index') else path
old2new={url_of(k):url_of(v) for k,v in m.items()}
files=[]
for dp,_,fs in os.walk(C):
    for f in fs:
        if f.endswith(('.md','.mdx')):
            p=os.path.relpath(os.path.join(dp,f),C); files.append(p)
def stem(p): return os.path.splitext(p)[0]
cur={}   # old file stem -> new file stem
for p in files:
    s=stem(p); cur[s]=m.get(s,s)
def newurl(u):
    return old2new.get(u,u)
def resolve(src_url,t):
    # src_url: url dir of page (no slashes at ends), t relative target -> absolute url path (no ends)
    # Source docs wrote relative links as if the page URL had no trailing slash,
    # so resolve against the PARENT directory of the old page URL.
    base=src_url.split('/') if src_url else []
    parts=base[:]
    for seg in t.split('/'):
        if seg in ('','.'): continue
        if seg=='..':
            if parts: parts.pop()
        else: parts.append(seg)
    return '/'.join(parts)
def rel(from_url,to_url):
    r=os.path.relpath(to_url or '.', from_url or '.')
    return r
all_old={url_of(stem(p)) for p in files}
report_unresolved=[]
LINK=re.compile(r'''(\]\(|href=")([^)"#\s]*)(#[^)"\s]*)?(\)|")''')
report=[]
def rewrite(text,old_url,new_url,fn):
    def sub(mm):
        pre,t,h,post=mm.group(1),mm.group(2),mm.group(3) or '',mm.group(4)
        if not t or re.match(r'^(https?:|mailto:|tel:|\.\./\.\./\.\./|data:)',t) : return mm.group(0)
        trail='/' if t.endswith('/') else ''
        if t.startswith('/'):
            if not t.startswith(BASE+'/'): return mm.group(0)
            absu=t[len(BASE)+1:].strip('/')
            n=newurl(absu)
            return pre+BASE+'/'+(n+'/' if n else '')+h.lstrip('') +post if False else pre+BASE+'/'+(n+'/' if n else '')+h+post
        t2=re.sub(r'\.mdx?$','',t.rstrip('/'))
        cands=[resolve('/'.join(old_url.split('/')[:-1]),t2).strip('/'), resolve(old_url,t2).strip('/')]
        absu=next((c for c in cands if c in all_old),None)
        if absu is None:
            report_unresolved.append((old_url,t)); return mm.group(0)
        n=newurl(absu)
        r=rel(new_url,n)
        if r=='.': r='' 
        out=r+('/' if r and not r.endswith('/') else '')
        if not out: out='./'
        return pre+out+h+post
    return LINK.sub(sub,text)
dry='--dry' in sys.argv
for p in files:
    s=stem(p); ns=cur[s]
    old_url=url_of(s); new_url=url_of(ns)
    full=os.path.join(C,p)
    text=open(full).read()
    new=rewrite(text,old_url,new_url,p)
    ext=os.path.splitext(p)[1]
    dest=os.path.join(C,ns+ext)
    if not dry:
        if ns!=s:
            os.makedirs(os.path.dirname(dest),exist_ok=True)
            subprocess.check_call(['git','mv',full,dest],cwd=ROOT)
        open(dest,'w').write(new)
    if new!=text: report.append(p)
print('unresolved',sorted(set(report_unresolved))[:30])
print('files',len(files),'changed',len(report),'moved',sum(1 for s in cur if cur[s]!=s))

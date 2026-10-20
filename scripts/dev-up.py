#!/usr/bin/env python3
"""Cross-platform launcher (#436)."""
from __future__ import annotations
import os, re, shutil, subprocess, sys, time
from pathlib import Path
REPO_ROOT = Path(__file__).resolve().parents[1]
BASE = ["-f", str(REPO_ROOT / "docker-compose.yml")]
AGENT = ["-f", str(REPO_ROOT / "docker-compose.yml"), "-f", str(REPO_ROOT / "docker-compose.agent.yml")]
IS_WINDOWS = os.name == "nt"
def host_home(): return Path(os.environ.get("HOST_HOME") or os.environ.get("HOME") or os.environ.get("USERPROFILE") or Path.home()).expanduser()
def opencode_bin(): p = Path(os.environ.get("OPENCODE_BIN","")).expanduser(); return p if str(p) else host_home()/".opencode/bin/opencode"
def is_elf(p): 
    try: return p.open("rb").read(4)==b"\x7fELF"
    except: return False
def agent_avail(): p = opencode_bin(); return p.is_file() and os.access(p,os.X_OK) and is_elf(p)
def env_g():
    e=os.environ.copy(); 
    if IS_WINDOWS or not hasattr(os,"getuid"): u,g="1000","1000"
    else: u,g=str(os.getuid()),str(os.getgid())
    e["HOST_UID"]=u; e["HOST_GID"]=g; t=""
    for n in ("GH_TOKEN","GITHUB_TOKEN"):
        t=os.environ.get(n,""); 
        if t: break
    if not t:
        try:
            gh=shutil.which("gh")
            if gh:
                r=subprocess.run([gh,"auth","token"],capture_output=True,text=True,timeout=5,check=False)
                if r.returncode==0: t=r.stdout.strip()
        except: pass
    if t: e["GH_TOKEN"]=e["GITHUB_TOKEN"]=t
    return e
def rc(argv, a=False):
    if not shutil.which("docker"): print("no docker"); raise SystemExit(1)
    f = AGENT if a and agent_avail() else BASE
    try: return subprocess.run(["docker","compose","--profile","dev",*f,*argv], cwd=REPO_ROOT, env=env_g(), check=False)
    except KeyboardInterrupt: return subprocess.CompletedProcess(["docker","compose"],130)
def main(a):
    if not a or a[0] in ("-h","--help"): print("dev-up|dev-down|dev-log|dev-exec|opencode|shell"); return 0
    c=a[0]; 
    if c=="dev-up": return rc(["up","-d","--build"]).returncode
    if c=="dev-down": return rc(["down"]).returncode
    if c=="dev-log": return rc(["logs","-f","dev"]).returncode
    if c=="dev-exec": return rc(["exec","dev","bash"]).returncode
    if c=="shell": return rc(["exec","-it","dev","bash"], True).returncode
    if c=="opencode": return rc(["exec","-it" if sys.stdin.isatty() else "-T","dev","bash","-lc","cd /workspace && opencode --auto"], True).returncode
    return 2
if __name__=="__main__": raise SystemExit(main(sys.argv[1:]))

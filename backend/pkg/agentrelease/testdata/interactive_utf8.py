import sys, shlex, os, pty, select, time, subprocess, threading, http.server, io, zipfile, hashlib
from pathlib import Path
root=Path(sys.argv[2])
stream=io.BytesIO()
with zipfile.ZipFile(stream,'w') as z:
 z.writestr('hephaestus-scan-agent',"#!/usr/bin/env bash\nprintf 'Hephaestus scan-agent v0.4.7 (cli mode)\\n'\n")
package=stream.getvalue()
class Server(http.server.BaseHTTPRequestHandler):
 def log_message(self,*args): pass
 def do_GET(self):
  if self.path.startswith('/api/v1/agent/release'): payload=b'v0.4.7\n'
  elif self.path.endswith('SHA256SUMS.txt'):
   machine=os.uname().machine
   arch='arm64' if machine in ['arm64','aarch64'] else 'amd64'
   operating='darwin' if os.uname().sysname=='Darwin' else 'linux'
   payload=(hashlib.sha256(package).hexdigest()+'  hephaestus-agent-'+operating+'-'+arch+'.zip\n').encode()
  else: payload=package
  self.send_response(200); self.end_headers(); self.wfile.write(payload)
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Server)
threading.Thread(target=server.serve_forever,daemon=True).start()
base='http://127.0.0.1:'+str(server.server_port)
master,slave=pty.openpty()
command=['bash',sys.argv[1],'--server',base,'--release-base',base+'/releases/download','--install-dir',str(root/'node'),'--no-start']
if os.uname().sysname=='Darwin': command=['script','-q','/dev/null',*command]
else: command=['script','-q','-c',shlex.join(command),'/dev/null']
process=subprocess.Popen(command,stdin=slave,stdout=slave,stderr=slave,env=dict(os.environ,no_proxy='127.0.0.1,localhost',NO_PROXY='127.0.0.1,localhost'))
os.close(slave)
buffer=b''
def wait_for(text):
 global buffer
 deadline=time.monotonic()+10
 while text not in buffer:
  assert time.monotonic()<deadline,'Timeout waiting for '+repr(text)
  if select.select([master],[],[],.2)[0]:
   try: buffer+=os.read(master,65536)
   except OSError: raise AssertionError('Installer exited before '+repr(text))
 buffer=buffer.split(text,1)[1]
def answer(prompt,value):
 wait_for(prompt);time.sleep(.03);os.write(master,value+b'\n')
try:
 prompt=b'PostgreSQL host (reachable from this node) []: '
 answer(prompt,b'10\xe30.211.55.2')
 wait_for(b'Invalid PostgreSQL host')
 answer(prompt,b'10.211.55.2')
 answer(b'PostgreSQL port [5432]: ',b'')
 answer(b'Database user [postgres]: ',b'')
 answer(b'Database password: ',b'bad\xff')
 wait_for(b'Invalid password encoding')
 password="测试'fixture".encode()
 answer(b'Database password: ',password)
 answer(b'Database name [hephaestus]: ',b'')
 answer(b'PostgreSQL sslmode (require / verify-full / disable) [disable]: ',b'')
 answer(b'Agent roles (scan / ai / ai,scan) [ai,scan]: ',b'')
 wait_for(b'Installed v0.4.7.')
 wait_for(b'Next update: ')
 # BSD script may remain attached after the installer exits; wait for cleanup.
 deadline=time.monotonic()+3
 while (root/'node/.install-lock').exists() and time.monotonic()<deadline: time.sleep(.05)
 assert not (root/'node/.install-lock').exists(), 'Installer did not finish cleanup'
 if process.poll() is not None: assert process.returncode==0
 cfg=(root/'node/config.yaml').read_bytes()
 cfg.decode('utf-8')
 assert b"host: '10.211.55.2'" in cfg
 assert b"user: 'postgres'" in cfg and b"sslmode: 'disable'" in cfg
 assert b"roles: ['ai', 'scan']" in cfg
 assert ("password: '测试''fixture'").encode() in cfg
 assert (root/'node/config.yaml').stat().st_mode & 0o777==0o600
 print('Interactive installer verified: invalid host/password re-prompted, Unicode/apostrophe password preserved, requested defaults and owner-only config correct.')
finally:
 os.close(master)
 if process.poll() is None: process.kill();process.wait(timeout=5)
 server.shutdown()

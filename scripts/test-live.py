#!/usr/bin/env python3
"""Read-only connection acceptance against >=2 already registered real PVE endpoints.
Use only on a deployment you administer. Password comes from environment or a
hidden prompt. Never disable HTTPS certificate verification.
"""
import getpass
import http.cookiejar
import json
import os
import sys
import urllib.error
import urllib.request
from urllib.parse import urlparse

base = os.environ.get('MMP_URL', '').rstrip('/')
if urlparse(base).scheme != 'https' or not urlparse(base).hostname:
    sys.exit('Set MMP_URL to the HTTPS manager origin')
email = os.environ.get('MMP_EMAIL') or input('Manager email: ')
password = os.environ.get('MMP_PASSWORD') or getpass.getpass('Manager password: ')
jar = http.cookiejar.CookieJar()
class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar), NoRedirect())
def request(path, data=None):
    headers = {'Content-Type': 'application/json', 'Origin': base}
    csrf = next((c.value for c in jar if c.name == 'mmp_csrf'), '')
    if csrf:
        headers['X-CSRF-Token'] = csrf
    req = urllib.request.Request(base + '/api/v1' + path,
        data=json.dumps(data).encode() if data is not None else None, headers=headers)
    with opener.open(req, timeout=30) as response:
        return json.load(response)
try:
    request('/auth/login', {'email': email, 'password': password})
    password = ''
    try:
        servers = request('/proxmox/servers')['items']
        if len(servers) < 2:
            raise RuntimeError('Register at least two independent PVE installations first')
        failed = False
        for server in servers:
            try:
                result = request('/proxmox/servers/' + server['id'] + '/test', {})
                print(json.dumps({'id': server['id'], 'status': result['status'],
                    'version': result['connection']['version'], 'nodes': result['connection']['node_count']}))
            except urllib.error.HTTPError as error:
                failed = True
                print(json.dumps({'id': server['id'], 'status': 'failed', 'http_status': error.code}))
        if failed:
            raise RuntimeError('One or more PVE connection tests failed')
    finally:
        request('/auth/logout', {})
except (urllib.error.URLError, RuntimeError, KeyError, ValueError):
    sys.exit('Acceptance failed; check manager logs and endpoint status. Credentials were not printed.')

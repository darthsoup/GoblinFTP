import contextlib
import ipaddress
import json
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import time
import unittest
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen
import uuid


IMAGE = sys.argv.pop(1) if len(sys.argv) > 1 else 'goblinftp:smoke'


def docker(*args, check=True):
    return subprocess.run(
        ['docker', *args], check=check, capture_output=True, text=True, timeout=60,
    )


def state(name):
    return json.loads(docker('inspect', '--format', '{{json .State}}', name).stdout)


def request(base, path, headers=None):
    try:
        response = urlopen(Request(base + path, headers=headers or {}), timeout=2)
    except HTTPError as error:
        response = error
    with response:
        return response.status, response.headers, response.read()


@contextlib.contextmanager
def container(*options, command=(), entrypoint=None):
    name = 'gftp-smoke-' + uuid.uuid4().hex[:12]
    args = [
        'run', '-d', '--name', name, '--read-only', '--cap-drop=ALL',
        '--security-opt=no-new-privileges:true', '--tmpfs=/tmp:rw,noexec,nosuid,size=16m',
        '-p', '127.0.0.1::8080', '-e', 'GFTP_LOG_LEVEL=debug', *options,
    ]
    if entrypoint:
        args.extend(['--entrypoint', entrypoint])
    if not any('/app/data' in option for option in options):
        args.extend(['-v', '/app/data'])
    try:
        docker(*args, IMAGE, *command)
        port = docker('port', name, '8080/tcp').stdout.strip().rsplit(':', 1)[1]
        yield name, 'http://127.0.0.1:' + port
    except Exception:
        logs = docker('logs', name, check=False)
        print(logs.stdout + logs.stderr, file=sys.stderr)
        raise
    finally:
        docker('rm', '-f', '-v', name, check=False)


def ready(name, base):
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        if not state(name)['Running']:
            raise AssertionError('Container exited before readiness')
        try:
            if request(base, '/healthz')[0] == 200:
                return
        except (URLError, TimeoutError, ConnectionError):
            pass
        time.sleep(0.2)
    raise AssertionError('Container did not become ready')


def exited(name):
    deadline = time.monotonic() + 12
    while time.monotonic() < deadline:
        current = state(name)
        if not current['Running']:
            return current['ExitCode']
        time.sleep(0.2)
    raise AssertionError('Container did not exit')


def last_ip(name):
    entries = []
    for line in docker('logs', name).stdout.splitlines():
        try:
            entry = json.loads(line)
        except ValueError:
            continue
        if entry.get('path') == '/api/auth/status':
            entries.append(entry)
    return entries[-1]['remote_ip']


class ContainerTests(unittest.TestCase):
    def test_routes_identity_and_graceful_shutdown(self):
        with container() as (name, base):
            ready(name, base)
            for route in ['/', '/login', '/edit']:
                status, headers, body = request(base, route)
                self.assertEqual(status, 200)
                self.assertEqual(headers['Cache-Control'], 'no-cache')
                self.assertEqual(headers['Content-Security-Policy'], "frame-ancestors 'none'")
                self.assertEqual(headers['X-Content-Type-Options'], 'nosniff')
                self.assertEqual(headers['Referrer-Policy'], 'no-referrer')
            asset = re.search(rb'(?:src|href)="(/_nuxt/[^"?]+\.js)', body).group(1).decode()
            status, headers, _ = request(base, asset)
            self.assertEqual(status, 200)
            self.assertIn('immutable', headers['Cache-Control'])
            self.assertEqual(request(base, '/_nuxt/missing.js')[0], 404)
            with urlopen(base + '/?sso=invalid', timeout=2) as response:
                self.assertIn('/login?sso_error=disabled', response.geturl())
            self.assertEqual(request(base, '/themes/missing/config.css')[0], 404)
            status, _, body = request(base, '/api/auth/status')
            self.assertEqual(status, 200)
            self.assertTrue(json.loads(body)['success'])
            peer = last_ip(name)
            self.assertFalse(ipaddress.ip_address(peer).is_loopback)
            request(base, '/api/auth/status', {'X-Forwarded-For': '203.0.113.9'})
            self.assertEqual(last_ip(name), peer)
            self.assertEqual(docker('exec', name, 'id', '-u').stdout.strip(), '10001')
            security = docker('exec', name, 'cat', '/proc/1/status').stdout
            self.assertRegex(security, r'CapEff:\s+0+\n')
            self.assertRegex(security, r'NoNewPrivs:\s+1\n')
            address = docker('exec', name, 'hostname', '-i').stdout.split()[0]
            probe = docker('exec', name, 'wget', '-T', '1', '-q', '-O', '/dev/null',
                           f'http://{address}:8081/healthz', check=False)
            self.assertNotEqual(probe.returncode, 0)
            docker('exec', name, 'wget', '-T', '2', '-q', '-O', '/dev/null',
                   'http://127.0.0.1:8080/healthz')
            docker('stop', '--time', '12', name)
            self.assertEqual(state(name)['ExitCode'], 0)

    def test_child_failure_stops_container(self):
        for process in ['gftp', 'caddy']:
            with self.subTest(process=process), container() as (name, base):
                ready(name, base)
                pid = docker('exec', name, 'pidof', process).stdout.split()[0]
                docker('exec', name, 'kill', '-TERM', pid)
                self.assertNotEqual(exited(name), 0)

    def test_slow_s3_does_not_block_startup(self):
        options = []
        for key, value in {
            'GFTP_S3_ENABLED': 'true', 'GFTP_S3_ENDPOINT': 'http://127.0.0.1:9000',
            'GFTP_S3_BUCKET': 'smoke', 'GFTP_S3_ACCESS_KEY': 'smoke', 'GFTP_S3_SECRET_KEY': 'smoke',
        }.items():
            options.extend(['-e', key + '=' + value])
        with container(*options, entrypoint='/bin/sh', command=(
            '-c', 'sleep 120 | nc -l -p 9000 >/dev/null & exec /entrypoint.sh',
        )) as (name, base):
            started = time.monotonic()
            ready(name, base)
            self.assertLess(time.monotonic() - started, 5)
            time.sleep(6)
            self.assertEqual(request(base, '/healthz')[0], 200)
            docker('stop', '--time', '12', name)
            self.assertEqual(state(name)['ExitCode'], 0)

    def test_invalid_configuration_never_serves_spa(self):
        with container('-e', 'GFTP_FRAME_ANCESTORS=invalid') as (name, base):
            self.assertNotEqual(exited(name), 0)
            self.assertIn('GFTP_FRAME_ANCESTORS', docker('logs', name).stdout)
            with self.assertRaises((URLError, TimeoutError, ConnectionError)):
                request(base, '/')

    def test_persistent_volume_permissions(self):
        volume = 'gftp-smoke-' + uuid.uuid4().hex[:12]
        docker('volume', 'create', volume)
        try:
            for iteration in range(2):
                with container('-v', volume + ':/app/data') as (name, base):
                    ready(name, base)
                    if iteration == 0:
                        docker('exec', name, 'sh', '-c', 'umask 077; printf pinned > /app/data/known_hosts')
                    self.assertEqual(docker('exec', name, 'cat', '/app/data/known_hosts').stdout, 'pinned')
                    docker('stop', '--time', '12', name)
            docker('run', '--rm', '--user', '0:0', '--entrypoint', 'sh',
                   '-v', volume + ':/app/data', IMAGE, '-c',
                   'chown -R 0:0 /app/data; chmod 700 /app/data')
            with container('-v', volume + ':/app/data') as (name, _):
                self.assertNotEqual(exited(name), 0)
                self.assertIn('ownership', docker('logs', name).stdout)
            docker('run', '--rm', '--user', '0:0', '--entrypoint', 'sh',
                   '-v', volume + ':/app/data', IMAGE, '-c', 'chown -R 10001:10001 /app/data')
            with container('-v', volume + ':/app/data') as (name, base):
                ready(name, base)
                self.assertEqual(docker('exec', name, 'cat', '/app/data/known_hosts').stdout, 'pinned')
        finally:
            docker('volume', 'rm', volume)

    def test_external_proxy_identity(self):
        network = 'gftp-smoke-' + uuid.uuid4().hex[:12]
        docker('network', 'create', network)
        try:
            with tempfile.TemporaryDirectory(prefix='gftp-smoke-') as directory:
                config = Path(directory) / 'Caddyfile'
                config.write_text('''{
    admin off
    persist_config off
}
:8080 {
    reverse_proxy app:8080 {
        header_up X-Forwarded-For "198.51.100.99, 203.0.113.9"
        header_up X-Forwarded-Proto https
    }
}
''')
                with container('--network', network, '-v', str(config) + ':/etc/caddy/Caddyfile:ro',
                               entrypoint='caddy', command=('run', '--config', '/etc/caddy/Caddyfile')) as (proxy, base):
                    proxy_ip = docker('inspect', '--format',
                                      '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}', proxy).stdout.strip()
                    with container('--network', network, '--network-alias', 'app',
                                   '-e', 'GFTP_ACCESS_TRUSTED_PROXIES=' + proxy_ip + '/32') as (name, direct):
                        ready(name, direct)
                        ready(proxy, base)
                        self.assertEqual(request(base, '/api/auth/status')[0], 200)
                        self.assertEqual(last_ip(name), '203.0.113.9')
                        request(direct, '/api/auth/status', {'X-Forwarded-For': '203.0.113.9'})
                        self.assertNotEqual(last_ip(name), '203.0.113.9')
        finally:
            docker('network', 'rm', network)


if __name__ == '__main__':
    unittest.main(verbosity=2)

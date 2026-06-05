<?php
// Generate a one-time GoblinFTP SSO login link. PHP >= 7.1.2, no extensions
// beyond OpenSSL (bundled in virtually every PHP build).
//
// Token format (must match backend/internal/sso/token.go):
//   key   = HKDF-SHA256(secret, salt = empty, info = "gftp-sso", length = 32)
//   token = base64url( iv(12) || gcmTag(16) || AES-256-GCM(key, iv, JSON payload) )
//
// CLI usage:
//   GFTP_SSO_SECRET=change-me php generate_sso_link.php \
//     --host=ftp.example.com --username=alice --password=s3cret \
//     --base-url=https://files.example.com
//
// Or call gftp_sso_link() from your own application code.

/**
 * Build a one-time SSO login URL for a GoblinFTP instance.
 *
 * @param string $secret  shared secret (the server's GFTP_SSO_SECRET)
 * @param string $baseUrl public URL of the GoblinFTP instance
 * @param array  $conn    protocol, host, port, username, password,
 *                        initialDirectory?, language?, ttlSeconds?
 */
function gftp_sso_link(string $secret, string $baseUrl, array $conn): string
{
    $protocol = $conn['protocol'] ?? 'ftp';
    $payload = [
        'type' => $protocol,
        'host' => $conn['host'],
        'port' => (int) ($conn['port'] ?? ($protocol === 'sftp' ? 22 : 21)),
        'username' => $conn['username'],
        'password' => $conn['password'],
        'initialDirectory' => $conn['initialDirectory'] ?? '',
        'language' => $conn['language'] ?? '',
        'exp' => time() + (int) ($conn['ttlSeconds'] ?? 300),
    ];

    $key = hash_hkdf('sha256', $secret, 32, 'gftp-sso', '');
    $iv = random_bytes(12);
    $tag = '';
    $ciphertext = openssl_encrypt(
        json_encode($payload, JSON_UNESCAPED_SLASHES),
        'aes-256-gcm',
        $key,
        OPENSSL_RAW_DATA,
        $iv,
        $tag
    );
    if ($ciphertext === false) {
        throw new RuntimeException('AES-256-GCM encryption failed');
    }

    // Wire format: iv || tag || ciphertext, base64url without padding.
    $token = rtrim(strtr(base64_encode($iv . $tag . $ciphertext), '+/', '-_'), '=');

    return rtrim($baseUrl, '/') . '/?sso=' . $token;
}

// ── CLI entry point ───────────────────────────────────────────────────────────
if (PHP_SAPI === 'cli' && realpath($argv[0] ?? '') === __FILE__) {
    $opts = getopt('', [
        'protocol::', 'host:', 'port::', 'username:', 'password::',
        'dir::', 'lang::', 'ttl-seconds::', 'base-url::', 'secret::',
    ]);

    $secret = $opts['secret'] ?? getenv('GFTP_SSO_SECRET');
    $password = $opts['password'] ?? getenv('GFTP_SSO_PASSWORD');
    if (!$secret || !isset($opts['host'], $opts['username'])) {
        fwrite(STDERR, "error: --host and --username are required; set --secret or GFTP_SSO_SECRET\n");
        exit(2);
    }

    echo gftp_sso_link($secret, $opts['base-url'] ?? 'http://localhost:8080', [
        'protocol' => $opts['protocol'] ?? 'ftp',
        'host' => $opts['host'],
        'port' => $opts['port'] ?? null,
        'username' => $opts['username'],
        'password' => $password === false ? '' : $password,
        'initialDirectory' => $opts['dir'] ?? '',
        'language' => $opts['lang'] ?? '',
        'ttlSeconds' => $opts['ttl-seconds'] ?? 300,
    ]) . "\n";
}

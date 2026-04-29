using System.Net;
using System.Text;

namespace PhantomManager;

internal sealed class NginxSslCertificateGenerator
{
    private readonly string _releaseDir;

    public NginxSslCertificateGenerator(string releaseDir)
    {
        _releaseDir = releaseDir;
    }

    public async Task CreateAsync(string ipAddress, Action<string>? log)
    {
        if (!IPAddress.TryParse(ipAddress, out var parsedAddress)
            || parsedAddress.AddressFamily != System.Net.Sockets.AddressFamily.InterNetwork)
        {
            throw new InvalidOperationException($"Invalid IPv4 address: {ipAddress}");
        }

        var script = new StringBuilder()
            .AppendLine("set -e")
            .AppendLine($"cd {WslCommand.PathArg(_releaseDir)}")
            .AppendLine("rm -f secrets/nginx/ca/phantom-local-ca.key secrets/nginx/ca/phantom-local-ca.crt secrets/nginx/ca/phantom-local-ca.srl secrets/nginx/ca/ca-openssl.cnf")
            .AppendLine("rm -f secrets/nginx/tls/privkey.pem secrets/nginx/tls/server-openssl.cnf secrets/nginx/tls/server.csr secrets/nginx/tls/fullchain.pem secrets/nginx/tls/fullchain.tmp")
            .AppendLine("mkdir -p secrets/nginx/ca secrets/nginx/tls")
            .AppendLine("openssl genrsa -out secrets/nginx/ca/phantom-local-ca.key 4096")
            .AppendLine("cat > secrets/nginx/ca/ca-openssl.cnf <<'EOF'")
            .AppendLine("[req]")
            .AppendLine("prompt = no")
            .AppendLine("default_md = sha256")
            .AppendLine("distinguished_name = dn")
            .AppendLine("x509_extensions = v3_ca")
            .AppendLine()
            .AppendLine("[dn]")
            .AppendLine("CN = phantom local CA")
            .AppendLine()
            .AppendLine("[v3_ca]")
            .AppendLine("subjectKeyIdentifier = hash")
            .AppendLine("authorityKeyIdentifier = keyid:always,issuer")
            .AppendLine("basicConstraints = critical, CA:TRUE")
            .AppendLine("keyUsage = critical, keyCertSign, cRLSign")
            .AppendLine("EOF")
            .AppendLine("openssl req -x509 -new -nodes \\")
            .AppendLine("  -key secrets/nginx/ca/phantom-local-ca.key \\")
            .AppendLine("  -sha256 -days 3650 \\")
            .AppendLine("  -config secrets/nginx/ca/ca-openssl.cnf \\")
            .AppendLine("  -extensions v3_ca \\")
            .AppendLine("  -out secrets/nginx/ca/phantom-local-ca.crt")
            .AppendLine("openssl genrsa -out secrets/nginx/tls/privkey.pem 2048")
            .AppendLine("cat > secrets/nginx/tls/server-openssl.cnf <<'EOF'")
            .AppendLine("[req]")
            .AppendLine("default_bits = 2048")
            .AppendLine("prompt = no")
            .AppendLine("default_md = sha256")
            .AppendLine("distinguished_name = dn")
            .AppendLine("req_extensions = req_ext")
            .AppendLine()
            .AppendLine("[dn]")
            .AppendLine($"CN = {parsedAddress}")
            .AppendLine()
            .AppendLine("[req_ext]")
            .AppendLine("basicConstraints = critical, CA:FALSE")
            .AppendLine("keyUsage = critical, digitalSignature, keyEncipherment")
            .AppendLine("extendedKeyUsage = serverAuth")
            .AppendLine("subjectAltName = @alt_names")
            .AppendLine()
            .AppendLine("[alt_names]")
            .AppendLine($"IP.1 = {parsedAddress}")
            .AppendLine("EOF")
            .AppendLine("openssl req -new \\")
            .AppendLine("  -key secrets/nginx/tls/privkey.pem \\")
            .AppendLine("  -out secrets/nginx/tls/server.csr \\")
            .AppendLine("  -config secrets/nginx/tls/server-openssl.cnf")
            .AppendLine("openssl x509 -req \\")
            .AppendLine("  -in secrets/nginx/tls/server.csr \\")
            .AppendLine("  -CA secrets/nginx/ca/phantom-local-ca.crt \\")
            .AppendLine("  -CAkey secrets/nginx/ca/phantom-local-ca.key \\")
            .AppendLine("  -CAcreateserial \\")
            .AppendLine("  -out secrets/nginx/tls/fullchain.pem \\")
            .AppendLine("  -days 825 -sha256 \\")
            .AppendLine("  -extensions req_ext \\")
            .AppendLine("  -extfile secrets/nginx/tls/server-openssl.cnf")
            .AppendLine("cat secrets/nginx/tls/fullchain.pem secrets/nginx/ca/phantom-local-ca.crt > secrets/nginx/tls/fullchain.tmp")
            .AppendLine("mv secrets/nginx/tls/fullchain.tmp secrets/nginx/tls/fullchain.pem")
            .ToString();

        await WslCommand.RunBashAsync(script.Replace("\r\n", "\n"), log);
    }
}

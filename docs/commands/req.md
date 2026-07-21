# req Online Help

[日本語](./req.ja.md)

`req` replays a raw HTTP request saved in a file, preserving its method, path, headers, and body. It is useful for reproducing proxy-captured requests or testing manually edited requests.

## Synopsis

```text
req [-S|--https] [-k|--no-tls-validation] [--tls-verify] <REQ_FILE>
```

```sh
req login.req
req --https login.req
req --no-tls-validation login.req
```

## Request File

Write a request line, headers, a blank line, and the optional body.

```http
POST /login HTTP/1.1
Host: target.thm
Content-Type: application/x-www-form-urlencoded
Cookie: session=example

username=admin&password=test
```

HTTP/1.0, HTTP/1.1, HTTP/2, and HTTP/2.0 request-line versions are accepted. Go's HTTP client performs transport protocol negotiation.

## URL Resolution

The request-target and `Host` header determine the destination. An absolute request URL supplies its own scheme and host. Otherwise `--https` selects HTTPS and HTTP is the default. IPv6 hosts and explicit ports are preserved. `Origin` and `Referer` are handled consistently with the final URL, while transport-generated or connection-specific headers such as `Content-Length` are normalized before sending.

## Options

- `-S, --https`: use HTTPS when the file does not determine a scheme.
- `-k, --no-tls-validation`: disable certificate verification for a known lab endpoint.
- `--tls-verify`: require certificate verification; cannot be combined with `-k`.
- `-h, --help`: show terminal help.
- `-V, --version`: print the executable version.

## Response and Exit Status

The HTTP response is written to stdout. Network, parsing, file, and TLS errors go to stderr and produce a non-zero exit status. `req` neither requires a ctx workspace nor registers responses automatically. To record it, use `ctx x req <file>` only after considering that output and arguments will be persisted.

## Security

Request files commonly contain cookies, authorization headers, passwords, and tokens. Do not accidentally commit them. Disabling TLS validation removes protection against man-in-the-middle attacks and should be limited to controlled lab environments.

## Troubleshooting

- Missing host: add a `Host` header or use an absolute request URL.
- HTTP is sent to an HTTPS endpoint: add `--https`.
- A self-signed lab certificate fails: verify the target, then use `-k`.
- Form credential enumeration is required: use `xhydra http` instead.

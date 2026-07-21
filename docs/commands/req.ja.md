# req オンラインヘルプ

[English](./req.md)

`req` は、ファイルに保存した生HTTPリクエストを読み込み、同じmethod、path、header、bodyで再送するCLIです。Proxyから保存したrequestの再現や、手作業で編集したHTTP requestの検証に使用します。

## 構文

```text
req [-S|--https] [-k|--no-tls-validation] [--tls-verify] <REQ_FILE>
```

```sh
req login.req
req --https login.req
req --no-tls-validation login.req
```

## Requestファイル

先頭行にrequest line、その後にheader、空行、bodyを記述します。

```http
POST /login HTTP/1.1
Host: target.thm
Content-Type: application/x-www-form-urlencoded
Cookie: session=example

username=admin&password=test
```

対応するrequest line versionはHTTP/1.0、HTTP/1.1、HTTP/2、HTTP/2.0です。送信時のprotocol negotiationはGo HTTP clientが行います。

## URLの決定

request lineのpathと `Host` headerから送信先を組み立てます。

- absolute URLがrequest targetに含まれる場合はそのscheme/hostを使用します。
- `--https` を指定すると、requestからschemeを決定できない場合にHTTPSを使用します。
- `--https` がなければscheme未指定時はHTTPを使用します。
- IPv6 hostや明示portもHost headerに従います。

`Origin` と `Referer` は、保存requestと最終URLの整合を保つように扱います。transportが自動生成する `Content-Length`、connection固有header、条件付きcache headerなどは再送時に整理されます。

## オプション

### `-S`, `--https`

requestファイルからschemeを決められない場合にHTTPSを強制します。既にabsolute URLが指定されている場合は、そのURLを優先します。

### `-k`, `--no-tls-validation`

TLS証明書の検証を無効化します。自己署名証明書を使う演習環境向けです。実運用の信頼できない通信先では、中間者攻撃を検出できなくなる点に注意してください。

### `--tls-verify`

TLS証明書を検証します。`-k` と同時には指定できません。

### `-h`, `--help`

端末内の簡易ヘルプを表示します。

### `-V`, `--version`

実行ファイルのversionを表示します。

## Response

送信後はHTTP responseを標準出力へ表示します。network error、invalid request、file read error、TLS errorは標準エラーと非0終了で通知します。

`req` はresponseをctxへ自動登録せず、workspaceにも依存しません。記録が必要な場合は、秘密情報がログへ残ることを理解した上で `ctx x req <file>` を使用できます。

## 安全上の注意

- requestファイルにはCookie、Authorization header、password、tokenが含まれる可能性があります。
- requestファイルをGitへ誤ってcommitしないでください。
- `ctx x` で実行するとresponseだけでなくcommand outputもctx databaseへ保存されます。
- `--no-tls-validation` は演習環境など、証明書を検証できない理由が明確な場合だけ使用してください。

## よくある問題

- `Host` がない: absolute URLをrequest lineへ書くか、Host headerを追加します。
- HTTPS endpointへHTTPで接続する: `--https` を指定します。
- 自己署名証明書で失敗する: 対象を確認して `-k` を指定します。
- `-k` と `--tls-verify` の併用エラー: どちらか一方だけを指定します。
- formの認証判定やwordlist試行が必要: `xhydra http` を使用します。

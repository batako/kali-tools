# Kali Tools

Kali Linux向けの自作CLIツールを管理するモノレポです。各ツールは独立したGoエントリポイントとDebianパッケージ定義を持ち、APTリポジトリとして公開します。

## ツール

- `req`: `.req` ファイルに保存した生HTTPリクエストを送信するCLI
- `ctx`: ワークスペース、ターゲット、サービス、credential、ノート、ログを管理するCLI
- `xssh`: ctxのcredentialを使って現在のターゲットへSSH接続するアドオン
- `xscp`: ctxのcredentialを使って現在のターゲットとローカル間でSSHファイル転送を行うアドオン
- `xftp`: ctxのcredentialを使って現在のターゲットへFTP接続するアドオン
- `xsmb`: SMB共有を一覧表示し、選択した共有へ接続するアドオン
- `xgobuster`: 現在のターゲットにGobusterを実行し、Web探索結果をctxへ保存するアドオン

`xssh`、`xscp`、`xftp`、`xsmb` はctxのアドオンです。ctxのJSON APIを利用し、ctxのSQLiteデータベースは直接参照しません。

## インストール

リポジトリを一度登録します。

```sh
echo "deb [trusted=yes] https://offsec.batako.net stable main" \
  | sudo tee /etc/apt/sources.list.d/batako-offsec.list
sudo apt update
```

必要なツールをインストールします。

```sh
sudo apt install req
sudo apt install ctx
sudo apt install xssh
sudo apt install xscp
sudo apt install xftp
sudo apt install xsmb
sudo apt install xgobuster
```

依存パッケージは各Debianパッケージで宣言しています。`xssh` と `xscp` は `openssh-client` と `sshpass`、`xftp` は `lftp`、`xsmb` は `smbclient`、`xgobuster` は `gobuster` と `wordlists` を使用します。

## 使い方

### req

リクエストファイルからHTTPメソッド、パス、Host、ヘッダー、ボディを読み込み、HTTPリクエストを再送します。

```sh
req login.req
req -S login.req
req -k login.req
```

`-S`/`--https` はリクエストファイルからスキームを決められない場合にHTTPSを強制します。期限切れや自己署名証明書を使う検証環境では`-k`/`--no-tls-validation`を使い、`--tls-verify`で明示的に検証を有効化できます。`-h`/`--help` と `-V`/`--version` も使用できます。リクエストファイルの `Accept-Encoding` と `Content-Length` は送信せず、gzipレスポンスの展開はGoの `net/http` に任せます。`req`は独立したDebianパッケージとして配布します。

### ctx

ワークスペースを作成・選択し、必要に応じてターゲットとcredentialを登録します。

```sh
ctx workspace init
ctx status
ctx target add 10.10.10.20 --name target
ctx credential set ssh root password
ctx scan
ctx service ls
ctx log
```

ほかに `ctx workspace ls`、`ctx workspace rm`、`ctx note`、`ctx prompt`、`ctx reset`、`ctx x <command> [args...]` などがあります。全コマンドは `ctx --help` で確認できます。

探索ツールは `/usr/share/wordlists` を自動的に探索します。`xgobuster` はインストール済みの `dirb`、`dirbuster`、`seclists` 配下からディレクトリ探索用のリストだけを選択します。パスワード、パラメータ、Fuzz用のリストは通常のディレクトリ探索から除外します。

```sh
sudo apt install wordlists seclists dirb dirbuster
ctx config set web.directory.max-requests 1000000
ctx config set web.file.max-requests 200000
ctx config set web.tls.verify false
```

シェル連携と `x` 系ヘルパーを使う場合:

```sh
ctx completion zsh
ctx completion bash
ctx init-shell
```

### アドオン

各アドオンはcredential IDまたはユーザー名を任意で受け取ります。credentialやサービスが複数ある場合は選択肢を表示します。該当するcredentialが登録されていない場合は、指定したユーザー名または通常のクライアントで接続します。

```sh
xssh
xssh root
xscp upload ./local.txt /tmp/remote.txt
xscp download /tmp/remote.txt ./local.txt
xftp
xftp ftpuser
xsmb
xsmb smbuser
```

接続開始・終了時刻、状態、終了コード、パスワードを含まないコマンド、stdout、stderrをctxのログに保存します。`xlog` で確認できます。パスワードはコマンドログへ保存しません。

`xsmb` はディスク共有を一覧表示し、`IPC$` を除外してから接続先を選択します。ctxに該当サービスがない場合、`xssh` は22番、`xftp` は21番、`xsmb` は445番を使用します。

`xscp upload` はローカルファイルをターゲットへ、`xscp download` はリモートファイルをローカルへ転送します。どちらも`xssh`と同じSSH credentialとサービス選択を利用します。

`xgobuster` は現在のターゲットからWebサービスを選択して `gobuster dir` を実行します。対象に`xhost`で手動登録したホスト名が1件なら自動利用し、複数ある場合はホスト名または対象IPを選択します。登録済みホスト名を明示する場合は`--host <hostname>`、対象IPを強制する場合は`--ip`、URL全体を指定する場合は`-u`または`--url`を使います。`/usr/share/wordlists` 配下からワードリストを自動選択し、設定したリクエスト数の上限まで実行します。`web.directory.max-requests` の既定値は1,000,000、`web.file.max-requests` の既定値は200,000です。ファイル検索では指定した拡張子分を含めた実リクエスト数で計算します。`web-quick`、`web-standard`、`web-deep`はディレクトリ検索とファイル検索で共通の探索強度です。`--next`で次の強度へ進み、`--force`で完了済みワードリストを再実行できます。`--status`で現在の検索状態を確認できます。`--preset`または`-x`を指定するとファイル検索になり、`-x`で拡張子を上書きできます。Gobusterは拡張子なしのパスも同時に検索するため、ディレクトリ検索と検索済み状態を共有し、同じパスを再送信しません。`-w`または`--wordlist`を指定した場合は、そのワードリストだけを使う手動検索になります。解析した探索結果はctxのログと探索データへ保存します。

`-c`または`--cookies`で、GobusterのリクエストへCookieを付与できます。共通の403レスポンスなどを除外する場合は`--exclude-length <size>`を使います。`xgo --sitemap`で、現在のターゲットから収集したパスを重複排除し、originごとにまとめてURL順で表示できます。端末出力ではHTTPステータスコードを色分けします。期限切れや自己署名証明書を使う検証環境では、`-k`または`--no-tls-validation`でTLS証明書検証を無効化できます。

`xgo` は `xgobuster` の短縮コマンドです。

`xgobuster` パッケージは、両コマンド用のbash・zsh補完ファイルもインストールします。インストール後に新しいシェルを起動するか、補完機能を再読み込みしてください。

プロファイルを指定してステータス表示やエスカレーション範囲を絞れます。

```sh
xgobuster --status --profile web-quick
xgobuster --next --profile web-standard
```

## ディレクトリ構成

```text
.
├── cmd/                 # Goコマンドのエントリポイント
├── internal/            # ツールの実装とテスト
├── debian/              # ツールごとのパッケージ定義とVERSION
├── scripts/             # ビルド、検証、公開スクリプト
├── releases/            # 英語・日本語のリリースノート
├── docs/                # ctxとAPIの詳細ドキュメント
├── .github/workflows/   # テスト、APT公開、リリースWorkflow
├── go.mod
└── README.md
```

各ツールのパッケージバージョンは `debian/<tool>/VERSION` で管理し、現在のバージョン番号はREADMEに重複して記載しません。

## ブランチと生成物

- `main`: ソースコード、テスト、パッケージ定義、スクリプト、ドキュメント
- `dev`: 開発用ブランチ。push時はテストのみ実行
- `apt-repo`: 生成済みAPTリポジトリ専用。AWS Amplifyはこのブランチを公開

以下は生成物のため、`main` にはコミットしません。

```text
dist/
repo/dists/
repo/pool/
```

## 開発

Kali開発コンテナ内でテストします。

```sh
docker-compose exec -w /tools kali go test ./...
docker-compose exec -w /tools kali gofmt -w cmd internal
```

ローカルでパッケージをビルドしてインストールする場合:

```sh
./scripts/install-deb.sh ctx
./scripts/install-deb.sh req
./scripts/install-deb.sh xssh
./scripts/install-deb.sh xftp
./scripts/install-deb.sh xsmb
```

対応アーキテクチャを指定したDebianパッケージの生成:

```sh
./scripts/build-deb.sh xssh amd64
./scripts/build-deb.sh xssh arm64
```

生成物は `dist/<tool>_<version>_<architecture>.deb` です。`scripts/build-apt-repo.sh` は `dist/` のパッケージを `repo/pool/` へコピーし、アーキテクチャごとの `Packages` と `Packages.gz` を再生成します。

## 自動化

`.github/workflows/test.yml` はpushごとに、Go moduleの整合性、テスト、ツールとパッケージのバージョン整合性を確認します。

`.github/workflows/publish-apt-repo.yml` は `main` へのpushでのみ実行します。テスト後、全ツールの `amd64` と `arm64` パッケージを生成し、APTリポジトリを再生成して、`repo/` の内容だけを `apt-repo` へforce pushします。チェックに失敗した場合は公開しません。

`.github/workflows/publish-release.yml` は次の形式のタグで実行します。

```text
ctx/v<version>
xssh/v<version>
xscp/v<version>
xftp/v<version>
xgobuster/v<version>
xsmb/v<version>
req/v<version>
xwebshell/v<version>
```

`releases/<tool>/<version>.md` の存在を確認し、`apt-repo` の対応パッケージを収集してGitHub Releaseを作成します。日本語版は対応する `.ja.md` です。

## ローカル検証

```sh
./scripts/check-version.sh xssh
./scripts/check-version.sh xscp
./scripts/check-version.sh xgobuster
./scripts/check-release.sh xssh
./scripts/check-published.sh xssh
```

`check-release.sh` はローカルKaliへのDebian/APTインストールを含む重い検証を行います。`check-published.sh` は公開後のAPTメタデータとパッケージを確認します。

## ドキュメント

ctxの詳細仕様、アーキテクチャ、データベース、API、アドオンの説明は `docs/` を参照してください。CLIの最新の構文は各コマンドの `--help` で確認します。

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
- `xffuf`: ffufとctxを使ってHTTP Virtual Hostとquery parameterを探索するアドオン
- `xhydra`: Hydraによる認証情報探索を補助し、結果をctxへ保存するアドオン
- `xwebshell`: Kali LinuxのWeb Shellテンプレートを選択・出力するCLI

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
sudo apt install xffuf
sudo apt install xhydra
sudo apt install xwebshell
```

## 使い方

### req

リクエストファイルからHTTPメソッド、パス、Host、ヘッダー、ボディを読み込み、HTTPリクエストを再送します。

```sh
req login.req
```

HTTPSやTLS検証などの指定は`req --help`で確認できます。

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

全コマンドと設定項目は`ctx --help`および`ctx config ls`で確認できます。

シェル連携と `x` 系ヘルパーを使う場合:

```sh
ctx completion zsh
ctx completion bash
ctx init-shell
```

### ctx連携ツール

接続、転送、探索でよく使う操作例:

```sh
xssh
xssh root
xscp upload ./local.txt /tmp/remote.txt
xscp download /tmp/remote.txt ./local.txt
xftp
xftp ftpuser
xsmb
xgo
xgo dns
xweb
xffuf vhost --suggest
xffuf param -u 'http://nahamstore.thm/?FUZZ=fuga'
xffuf param -u 'http://nahamstore.thm/?hoge=FUZZ'
xweb --type param
xhydra --help
xwebshell ls
```

各ツールはctxのターゲット、サービス、credentialなどを必要に応じて利用します。実行方法と利用可能なオプションは各コマンドの`--help`で確認してください。

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
./scripts/install-deb.sh <tool>
```

## ドキュメント

- CLIの構文: 各コマンドの`--help`
- 開発・検証・リリース手順: [開発ガイド](docs/development.ja.md)
- カスタムコマンドとの連携: [ctx連携ガイド](docs/integration.ja.md)
- JSON API: [ctx JSON API](docs/api.ja.md)
- データベース: [データベース設計](docs/database.ja.md)
- 登録コマンド: [ctx登録コマンド](docs/registration.ja.md)

# Kali Tools

Kali Linux向けの自作CLIツールを管理するモノレポです。各ツールは独立したGoエントリポイントとDebianパッケージ定義を持ち、APTリポジトリからインストールできます。

## ツール

使い方と仕様はコマンドドキュメントで管理します。

- [req](docs/commands/req.ja.md)
- [ctx](docs/commands/ctx.ja.md)
- [xssh](docs/commands/xssh.ja.md)
- [xscp](docs/commands/xscp.ja.md)
- [xftp](docs/commands/xftp.ja.md)
- [xsmb](docs/commands/xsmb.ja.md)
- [xgobuster](docs/commands/xgobuster.ja.md)
- [xffuf](docs/commands/xffuf.ja.md)
- [xhydra](docs/commands/xhydra.ja.md)
- [xwebshell](docs/commands/xwebshell.ja.md)
- [xmagic](docs/commands/xmagic.ja.md)
- [xsteg](docs/commands/xsteg.ja.md)

## インストール

リポジトリを一度登録します。

```sh
echo "deb [trusted=yes] https://offsec.batako.net stable main" \
  | sudo tee /etc/apt/sources.list.d/batako-offsec.list
sudo apt update
```

ツール一式をインストールします。

```sh
sudo apt install batako-kali-tools
```

必要なツールだけを個別にインストールすることもできます。

```sh
sudo apt install <package> [<package> ...]
```

## ディレクトリ構成

```text
.
├── cmd/                 # Goコマンドのエントリポイント
├── internal/            # ツールの実装とテスト
├── debian/              # ツールごとのパッケージ定義とVERSION
├── scripts/             # ビルド、検証、公開スクリプト
├── releases/            # 英語・日本語のリリースノート
├── docs/
│   └── commands/        # コマンド別の日本語・英語ドキュメント
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
go test ./...
gofmt -w cmd internal
```

ローカルでパッケージをビルドしてインストールする場合:

```sh
./scripts/install-deb.sh <tool>
```

## ドキュメント

- コマンドの使い方と仕様: [コマンドドキュメント](docs/commands/)
- 開発・検証・リリース手順: [開発ガイド](docs/development.ja.md)
- カスタムコマンドとの連携: [ctx連携ガイド](docs/integration.ja.md)
- JSON API: [ctx JSON API](docs/api.ja.md)
- データベース: [データベース設計](docs/database.ja.md)

# Kali Tools

Kali Linux 向けの自作 CLI ツールを管理するリポジトリです。

## 現在のツール

- `req`: `.req` ファイルとして保存した生 HTTP リクエストを送信する CLI

## req の使い方

```sh
req <REQ_FILE>
req -S <REQ_FILE>
req --help
```

オプション:

- `-S`, `--https`: リクエストファイルからスキームを決められないときに `https` を強制する
- `-h`, `--help`: ヘルプを表示する

## ディレクトリ構成

- `cmd/req/`: `req` のエントリポイント
- `internal/req/`: `req` の実装とテスト
- `debian/req/`: `req` の Debian パッケージ定義
- `scripts/`: ビルドと公開用スクリプト
- `.github/workflows/`: GitHub Actions

## ブランチの役割

- `main`: ソースコード管理用
- `apt-repo`: 公開用 APT リポジトリ

`apt-repo` には生成済みの APT リポジトリだけを置き、AWS Amplify はこのブランチだけを公開対象にします。

## main で管理しない生成物

- `dist/`
- `repo/dists/`
- `repo/pool/`

これらは `.gitignore` に入っており、`main` にはコミットしません。

## GitHub Actions

### `test.yml`

すべての `push` で次を実行します。

```sh
go mod tidy
git diff --exit-code
go test ./...
```

### `publish-apt-repo.yml`

`main` への `push` のときだけ次を順に実行します。

```text
go mod tidy
git diff --exit-code
go test ./...
./scripts/build-deb.sh
./scripts/build-apt-repo.sh
apt-repo ブランチへ force push
```

テストまたは tidy 差分チェックに失敗した場合は公開しません。

## Debian パッケージ生成

```sh
./scripts/build-deb.sh
```

生成物:

```text
dist/req_<version>_<architecture>.deb
```

補足:

- アーキテクチャは `dpkg --print-architecture` で取得します
- Go の `GOARCH` は Debian アーキテクチャに合わせて変換します
- バージョンは `debian/req/VERSION` から読み込みます

## APT リポジトリ生成

`.deb` 生成後に実行します。

```sh
./scripts/build-apt-repo.sh
```

生成物:

```text
repo/dists/stable/main/binary-all/Packages
repo/dists/stable/main/binary-all/Packages.gz
repo/dists/stable/main/binary-amd64/Packages
repo/dists/stable/main/binary-amd64/Packages.gz
repo/dists/stable/main/binary-arm64/Packages
repo/dists/stable/main/binary-arm64/Packages.gz
repo/pool/main/r/req/req_<version>_amd64.deb
repo/pool/main/r/req/req_<version>_arm64.deb
```

## APT リポジトリの利用

リポジトリを追加します。

```sh
echo "deb [trusted=yes] https://apt.batako.net stable main" \
| sudo tee /etc/apt/sources.list.d/batako.list
```

パッケージ一覧を更新します。

```sh
sudo apt update
```

`req` をインストールします。

```sh
sudo apt install req
```

リポジトリを削除する場合:

```sh
sudo rm -f /etc/apt/sources.list.d/batako.list
sudo apt update
```

# Kali Tools

Kali Linux 向けの自作 CLI ツールを管理するリポジトリです。

## 現在のツール

- `req`: `.req` ファイルとして保存した生 HTTP リクエストを送信する CLI
- `ctx`: ターゲットと hosts のワークスペースコンテキストを管理する CLI

## req の使い方

```sh
req <REQ_FILE>
req -S <REQ_FILE>
req --help
req --version
req -V
```

オプション:

- `-S`, `--https`: リクエストファイルからスキームを決められないときに `https` を強制する
- `-h`, `--help`: ヘルプを表示する
- `-V`, `--version`: バージョンを表示する

## ctx の使い方

```sh
ctx workspace init
ctx status
ctx workspace ls
ctx workspace rm [id]
ctx note "SMB anonymous login possible"
ctx log
ctx log <id>
ctx prompt
ctx prompt --field target-ip
ctx prompt --format json
ctx x <command> [args...]
ctx --help
ctx --version
ctx -V
x <command> [args...]
```

`ctx x` は現在の ctx ワークスペース内で指定したコマンドを実行し、stdout/stderr を端末へ流しながら、実行コマンド、展開後コマンド、終了コード、時刻、stdout、stderr を `ctx log` に保存します。引数に `$IP` または `${IP}` が含まれる場合は、実行前に現在の primary target IP へ展開します。`ctx init-shell` 後は、`x` helper function を `ctx x` の短縮形として使えます。

`ctx note <text>` はノートを `note:<id>` として `ctx log` のタイムラインへ保存します。`ctx init-shell` 後は短縮形の `xnote <text>` を使えます。

端末で `ctx log` を実行すると対話型タイムラインが開きます。`j`/`k` または矢印キーで移動し、Enterでコマンド詳細を開き、`q`で戻るか終了します。コンパクトなテキスト表示には `-p`/`--plain`、IDや実行状態を含む表示には `-v`/`--verbose`、明示的にTUIを開く場合は `-i`/`--interactive` を使います。

`ctx prompt` はプロンプト連携用に、安全にクォートしたシェル変数を出力します。ワークスペース、ローカルインターフェース/IP、primary targetの情報を含みます。ワークスペース外では `CTX_ACTIVE` が `0` になります。`.p10k.zsh` で使う最小限のPowerlevel10kカスタムセグメント例は次のとおりです。

```zsh
function prompt_ctx() {
  eval "$(ctx prompt)" || return
  (( CTX_ACTIVE )) || return
  p10k segment -t "${CTX_LOCAL_IP} -> ${CTX_TARGET_IP}"
}
```

`POWERLEVEL9K_LEFT_PROMPT_ELEMENTS` または `POWERLEVEL9K_RIGHT_PROMPT_ELEMENTS` に `ctx` を追加し、色、アイコン、表示形式は必要に応じてセグメント側で設定します。単一の値は `ctx prompt --field <name>`、構造化データは `ctx prompt --format json` で取得できます。

`ctx workspace rm` は確認後、現在のワークスペースのマーカー、DBレコード、データディレクトリを削除します。ワークスペース外では登録済み一覧から選択できます。IDを指定すれば直接選択でき、`--yes` を付けると確認を省略します。

## ctx のシェル初期設定

```sh
ctx completion zsh
ctx completion bash
ctx init-shell
ctx init-shell --remove
ctx doctor
```

`ctx completion zsh` と `ctx completion bash` はシェルスクリプトを標準出力に出すだけで、rc ファイルは変更しません。

`ctx init-shell` は現在のシェルを判定し、`.zshrc` または `.bashrc` に ctx 用のマーカー付きブロックを追記します。あわせて x プレフィックスの helper function も有効になるため、`ctx workspace init` は `xinit`、`ctx status` は `xstatus`、`ctx hosts` は `xhosts` として実行できます。ctx は alias を作成しません。

## ディレクトリ構成

- `cmd/req/`: `req` のエントリポイント
- `internal/req/`: `req` の実装とテスト
- `debian/req/`: `req` の Debian パッケージ定義
- `debian/ctx/`: `ctx` の Debian パッケージ定義
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
./scripts/check-version.sh ctx
```

### `publish-apt-repo.yml`

`main` への `push` のときだけ次を順に実行します。

```text
go mod tidy
git diff --exit-code
go test ./...
./scripts/check-version.sh ctx
./scripts/build-deb.sh req amd64
./scripts/build-deb.sh req arm64
./scripts/build-deb.sh ctx amd64
./scripts/build-deb.sh ctx arm64
既存の apt-repo ブランチを repo/ に復元
./scripts/build-apt-repo.sh
apt-repo ブランチへ force push
```

テストまたは tidy 差分チェックに失敗した場合は公開しません。

## Debian パッケージ生成

```sh
./scripts/build-deb.sh
./scripts/build-deb.sh ctx
```

パッケージ名とアーキテクチャを明示することもできます。

```sh
./scripts/build-deb.sh req amd64
./scripts/build-deb.sh req arm64
./scripts/build-deb.sh ctx amd64
./scripts/build-deb.sh ctx arm64
```

生成物:

```text
dist/<package>_<version>_<architecture>.deb
```

補足:

- アーキテクチャは `dpkg --print-architecture` で取得します
- Go の `GOARCH` は Debian アーキテクチャに合わせて変換します
- パッケージを省略した場合は `req` を生成します
- `ctx` パッケージに入る実行ファイルは `ctx` です。`x` は shell integration が提供する `ctx x` 用 helper です
- バージョンは `debian/<package>/VERSION` から読み込みます
- `./scripts/check-version.sh ctx` で `debian/ctx/VERSION` と `internal/ctx.Version` の一致を確認します

## リリースチェック

リリース前は、バージョン、Go module、テスト、Debian パッケージとパッケージ内の実行ファイルをまとめて確認します。さらに実行中の Kali Linux へ `.deb` をAPTインストールし、基本動作、`postinst`、アンインストールを確認してから再インストールします。この確認では `sudo` による管理者権限が必要です。APTインストール後、インストール済みのctxで現在の `.zshrc` または `.bashrc` に対する削除、登録、重複防止、更新、読込を検証します。全項目成功時はctx設定を残し、失敗または中断した場合だけ元の内容と更新日時を復元します。対話端末で成功すると設定読込済みのシェルを起動するため、そのままTab補完などを確認できます。

```sh
./scripts/check-release.sh ctx
```

リリース公開後は、`apt.batako.net` の amd64/arm64 用 APT メタデータと `.deb` を確認します。反映待ちを考慮して HTTP 取得は自動で再試行します。公開APTリポジトリからの更新とバージョン指定インストールは、実行後に `TODO` として表示されます。

```sh
./scripts/check-published.sh ctx
```

別のリポジトリを確認する場合:

```sh
APT_REPOSITORY_URL=https://example.net ./scripts/check-published.sh ctx
```

## APT リポジトリ生成

`.deb` 生成後に実行します。新しいパッケージを `dist/` から `repo/pool/` へコピーし、既存パッケージは削除しません。その後、`repo/pool/` に保存されているすべての `.deb` から `dpkg-scanpackages --multiversion` でメタデータを再生成します。

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
repo/pool/main/c/ctx/ctx_<version>_amd64.deb
repo/pool/main/c/ctx/ctx_<version>_arm64.deb
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

最新版の `req` または `ctx` をインストールします。

```sh
sudo apt install req
sudo apt install ctx
```

バージョンを指定してインストールする場合:

```sh
sudo apt install ctx=0.1.0
sudo apt install req=0.2.3
```

リポジトリを削除する場合:

```sh
sudo rm -f /etc/apt/sources.list.d/batako.list
sudo apt update
```

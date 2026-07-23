# xdec

`xdec` は、文字列、ファイル、stdin を一つの入力モデルで扱うデコード・復元フロントエンドです。引数なしではルートヘルプを表示し、入力がある場合はデコードと復元を自動判定します。

## 使い方

```sh
# ルートヘルプ
xdec
xdec help
xdec --help

# バージョン
xdec version
xdec -V
xdec --version

# 文字列を直接渡す
xdec decode 'QXJlYTUx'
xdec decode --string 'QXJlYTUx'
xdec 'QXJlYTUx'

# -f でファイルを渡す
xdec recover -f hashes.txt

# 実在する通常ファイルは -f を省略できる
xdec ~/.ssh/id_ed25519

# stdin
some-command | xdec recover --yes -w wordlist.txt
```

位置引数の後ろにオプションを置く書き方も利用できます。

```sh
xdec recover ~/.ssh/id_ed25519 --refresh --yes -w wordlist.txt
```

## サブコマンド

| サブコマンド | 説明 |
| --- | --- |
| `decode` | 復元を開始せずに値をデコード |
| `recover` | パスワード・鍵のパスフレーズを復元 |
| `help [SUBCOMMAND]` | ルートまたは指定したサブコマンドのヘルプを表示 |
| `version` | バージョンを表示 |

## ルートオプション

| オプション | 説明 |
| --- | --- |
| `-h`, `--help` | ルートヘルプを表示 |
| `-V`, `--version` | バージョンを表示 |
| `--online-help` | バージョン付きオンラインヘルプ URL を表示 |

## decode / recover の引数とオプション

位置引数が実在する通常ファイルならファイルとして読み、それ以外は文字列として扱います。判定が曖昧な場合は、`--file` または `--string` で型を明示できます。位置引数は1個だけ指定でき、型指定フラグとは併用できません。入力指定がなければ stdin を読みます。

| オプション | 説明 |
| --- | --- |
| `-f`, `--file FILE` | 入力ファイル。既存ファイルの位置引数でも代用可能 |
| `--string VALUE` | VALUE を文字列として扱う。位置引数との併用不可 |
| `--json` | 結果を JSON で出力 |
| `-h`, `--help` | decode のヘルプを表示 |

`recover` は同じ入力形式に加えて、wordlist、確認、state、credential 保存用のオプションを受け付けます。

| オプション | 説明 |
| --- | --- |
| `-f`, `--file FILE` | 入力ファイル。既存ファイルの位置引数でも代用可能 |
| `--string VALUE` | VALUE を文字列として扱う。位置引数との併用不可 |
| `-w`, `--wordlist SPEC` | ctx の wordlist ID またはパス。複数指定可能 |
| `--scope SCOPE` | credential 保存時の scope |
| `--username USER` | 入力にユーザ名がない場合のユーザ名 |
| `--save-credential` | 復元した credential を ctx に保存 |
| `--no-save-credential` | credential の自動保存を無効化 |
| `--yes` | 高コスト解析を承認 |
| `--refresh` | 現在の入力の保存 state を破棄して再解析 |
| `--dry-run` | 実行計画だけ表示 |
| `--json` | 結果を JSON で出力 |
| `-h`, `--help` | recover のヘルプを表示 |

## 解析の流れ

Base64 / hex は即時にデコードします。ルートコマンドは復元対象の hash や暗号化 SSH 鍵を自動的に復元処理へ振り分けます。明示した `decode` は復元を開始せず、復元が必要であることを表示します。明示した `recover` は復元対象だけを処理します。

高コスト解析は、既定では ctx の password wordlist 集合を使います。`-w` を複数指定した場合は、最初の wordlist で解決しなければ次へ自動継続します。確認画面では大量のパスを列挙せず、wordlist 数だけを表示します。

パイプ入力など非対話 stdin では確認に回答できないため、`--yes` を指定してください。

```sh
cat md5.txt | xdec recover --yes -w wordlist.txt
```

## SSH 秘密鍵

`id_rsa`、`id_ecdsa`、`id_ed25519` などの秘密鍵ファイルを直接渡せます。暗号化状態を自動判定し、未暗号化鍵は John を実行せず、次のように終了します。

```text
xdec: SSH private key is not encrypted
xdec: no password required
```

暗号化鍵だけを `ssh2john` に変換し、John でパスフレーズを解析します。秘密鍵の内容や復元パスフレーズは xlog に記録しません。

```sh
xdec recover --yes -w wordlist.txt -f ~/.ssh/id_rsa
```

## ユーザ名と credential

`admin:HASH` と `admin HASH` の両方からユーザ名を抽出します。

```sh
cat command-output.txt | xdec recover --scope ssh --yes -w wordlist.txt
```

出力には scope を含めず、ユーザ名だけを表示します。

```text
admin: password
alice: password
```

username と scope が確定した復元結果は ctx credential に自動保存します。純粋な hash や scope 不明の結果は自動保存しません。

## state とログ

実行履歴は `xlog` / `ctx log` で確認できます。xdec の親ログの下に、wordlist ごとの子 step が作成されます。

同じ入力を再実行すると、完了済みの wordlist はスキップし、保存済みの復元結果を表示します。新しい wordlist を追加した場合は、その wordlist だけを続きから試します。

```sh
# 保存状態を破棄して、この入力だけ再解析
xdec recover --refresh --yes -w wordlist.txt -f bcrypt.txt
```

state は ctx workspace の `data/xdec/state.json`（workspace 外ではユーザ cache）に 0600 で保存されます。復元済みの平文を含むため、機密情報として扱ってください。

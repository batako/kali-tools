# ctx オンラインヘルプ

[English](./ctx.md)

`ctx` は、TryHackMe、Hack The Boxなどの演習環境で、調査対象と実行履歴をワークスペース単位に管理するためのCLIです。ターゲット、ホスト名、検出サービス、credential、Web探索結果、ワードリストの進捗、ノート、コマンドログを一貫したコンテキストとして保持します。

## 基本構文

```text
ctx <command> [options]
```

```sh
ctx workspace init
ctx target add 10.10.10.20 --name target
ctx scan
ctx service ls
ctx log
```

`ctx` は現在のディレクトリから親方向へ `.ctx` マーカーを探し、所属ワークスペースを決定します。多くの操作はワークスペース内で実行する必要があります。

## ワークスペース

### `ctx workspace init`

現在のディレクトリをワークスペースとして初期化します。`.ctx` には正規化されたUUIDだけを保存します。SQLiteデータベースとツール状態はctxのデータディレクトリで管理されます。

### `ctx workspace ls`

登録済みワークスペースを一覧表示します。

### `ctx workspace rm [id] [-y|--yes]`

対象ワークスペースのctx管理データとマーカーを削除します。確認を省略する場合は `--yes` を指定します。調査ディレクトリ内の一般ファイルを一括削除するコマンドではありません。

## Project管理

Projectは、設定したroot配下にワークスペースを作るための任意機能です。任意ディレクトリでの `ctx workspace init` はProjectを使わなくても利用できます。

```text
ctx project root [path]
ctx project root add <path> [--name <name>]
ctx project root use <name>
ctx project root ls
ctx project root rm <name>
ctx project root move <from> <to> [--dry-run] [-y|--yes]
ctx project new <name>
ctx project ls
ctx project rm <id|name> [-y|--yes]
```

- `root` は現在使用するProject rootを表示または変更します。
- `root add` は名前付きrootを登録します。`--name` を省略するとパス末尾から名前を決めます。
- `root move` は登録済みroot間でctx管理Projectを移動します。事前確認には `--dry-run` を使用します。
- `project <name>` は `project new <name>` の短縮形です。

## TargetとHost

### Target

```text
ctx target set <ip>
ctx target add <ip> [--name <name>]
ctx target update <ip>
ctx target use <name>
ctx target rm <name>
ctx target ls
```

Targetは攻撃対象を表します。複数登録できますが、アドオンが既定で使用するPrimary Targetは1件です。

- `ctx target <ip>` は `ctx target set <ip>` と同じです。
- `ctx ip` はPrimary TargetのIPを表示します。
- `ctx ip <ip>` はPrimary TargetのIPを更新します。

### Hostname

```text
ctx host add <hostname> [--target <name>]
ctx host rm <hostname>
ctx host ls
ctx host <hostname>
```

引数だけを指定した `ctx host <hostname>` は `host add` と同じです。Web探索ツールは登録済みhostnameをTarget IPより優先して候補にできます。

### `/etc/hosts` 連携

```text
ctx hosts show
ctx hosts sync [--internal]
ctx hosts clean [--internal]
```

- `show` はctx管理ブロックを表示します。
- `sync` は登録済みTargetとHostから `/etc/hosts` の管理ブロックを同期します。必要に応じてsudoで再実行します。
- `clean` はctx管理ブロックだけを削除します。
- `--internal` はsudo再実行用の内部オプションで、通常は指定しません。

## ScanとService

```text
ctx scan [ip] [-p|--ports <ports>] [-n|--dry-run] [-f|--force]
ctx service ls [--target <name>] [--format <shell|json>] [--format-version <version>]
```

`ctx scan` はNmapを `-Pn -n -sV` で実行し、通常出力とXMLを保存して、open portとservice情報をctxへ登録します。

- `--ports` はNmapの `-p` に渡すポート指定です。
- `--dry-run` は実行せず展開後コマンドだけを表示します。
- `--force` がなければ、同じTarget、IP、port条件で成功済みの重複scanを省略します。
- IPを直接指定した場合、そのIPを対象にします。

`service ls` はPrimary Target、または `--target` で指定したTargetの保存済みserviceを表示します。外部連携では人間向け表を解析せず、version付きJSONを使用してください。

## Credential

```text
ctx credential ls [scope]
ctx credential set <scope> <username> [password]
ctx credential add <scope> <username> [password]
ctx credential update <scope> <username> [password]
ctx credential rm <id|username|scope username> [-y|--yes]
```

scopeには `ssh`、`ftp`、`smb` など、利用先を識別する名前を指定します。引数なしのpasswordは、パスワードを持たないcredentialとして保存できます。

```sh
ctx credential set ssh root toor
ctx credential set ftp anonymous
ctx credential ls ssh
```

`ctx credential <scope> <username> [password]` は `credential set` の短縮形です。JSON出力には平文passwordが含まれるため、ログや共有ファイルへ保存しないでください。

## Web探索結果

```text
ctx web ls [--target <name>] [--type <type>] [--format <shell|json>] [--format-version <version>]
ctx web show <id> [--target <name>]
ctx web clear [--target <name>]
```

typeは次のいずれかです。

- `path`: directory/file探索結果
- `param`: parameter探索結果全体
- `param-name`: query parameter名の探索結果
- `param-value`: query parameter値の探索結果

`clear` は選択TargetのWeb探索結果、wordlist実行履歴、xgobuster/xffufの検索済みword cacheを確認後に削除します。command logや無関係なTargetの状態は削除しません。

## Wordlist

```text
ctx wordlist [ls] [path] [--kind <kind>] [--usable-only] [--format <table|json|markdown>]
ctx wordlist show <ID|path> [path]
ctx wordlist extract [-y|--yes] [--force] [--remove-source]
```

引数なしの `ctx wordlist` は `ctx wordlist ls` と同じです。既定では `/usr/share/wordlists` を再帰走査し、symlink先、provider、形式、圧縮状態、利用可能性、既知/未知分類、用途別優先度を表示します。

利用可能なkind:

- `all`
- `directory`
- `subdomain`
- `parameter-name`
- `parameter-value`
- `password`
- `username`
- `endpoint`
- `unknown`

`--kind` を指定した場合、既知で適合度の高いwordlistを先頭にし、用途別priority順で返します。`--usable-only` は実行可能な項目だけに絞ります。xffuf、xgobuster、xhydra、xstegはこの推薦順を共通利用します。

`wordlist extract` は、ctxに埋め込まれたSHA-256と一致する `/usr/share/wordlists/rockyou.txt.gz` だけを検証して展開します。`--force` は既存出力の置換、`--remove-source` は成功後の圧縮元削除、`--yes` は確認省略です。書き込み権限が必要な場合はctxがsudo再実行を案内します。

## Timeline、Note、Log

```text
ctx note <text>
ctx log [id] [-p|--plain|-v|--verbose|-i|--interactive]
```

`note` は現在のワークスペースtimelineへメモを追加します。

`log` はnoteとcommand logを時系列表示します。

- `--plain`: 時刻と操作だけの簡潔な一覧
- `--verbose`: ID、状態、終了コードを含む一覧
- `--interactive`: キー操作で選択・詳細表示するTUI
- `ctx log <id>`: command、展開後command、状態、stdout、stderrを表示

親子ログを持つコマンドは、通常一覧では親だけを表示し、詳細の `steps` で内部コマンドを確認できます。外部アドオン向けの `log start` と `log finish` はversion付きJSONを標準入力から受け取ります。

## 任意コマンドの実行

```text
ctx x <command> [args...]
```

子プロセスをshell経由ではなく直接起動し、stdout/stderrを端末へ流しながらcommand logへ保存します。各引数中の `$IP` と `${IP}` はPrimary Target IPに置換されます。

```sh
ctx x nmap -sV '$IP'
ctx x curl "http://${IP}/"
```

pipeやredirectを使う場合は、`ctx x sh -c '...'` のようにshellを明示してください。引数と出力はログへ保存されるため、秘密情報を含むコマンドには使用しないでください。

## ShellとJSON連携

```text
ctx prompt [--format <shell|json>] [--format-version <version>] [--field <name>]
ctx formats [--format <shell|json>] [--format-version <version>]
ctx completion <zsh|bash> [--extra-shortcuts]
ctx init-shell [--remove|--extra-shortcuts]
```

`prompt` はworkspace、local IP/interface、Primary TargetをshellまたはJSONで返します。単一値には `--field` を使用できます。

field:

```text
active workspace-id workspace-name workspace-path
local-ip local-interface target-name target-ip
```

`formats` は利用可能なJSON endpointとformat versionを列挙します。外部ツールはctxのパッケージversionではなく、この出力で必要APIの対応を確認してください。

`init-shell` は現在のshellへ補完とshortcutを設定します。通常shortcut:

```text
xinit xconfig xworkspace xstatus xproject xnew xtarget xip xhost xhosts
xscan xservice xweb xwordlist xcredential xnote xlog xprompt xformats
x xcompletion xdoctor xinit-shell xreset
```

`--extra-shortcuts` では `pj`、`ta`、`cr`、`sv` も追加します。

## Config

```text
ctx config ls
ctx config get <key>
ctx config set <key> <value>
```

主な設定:

- `project.root`: active Project root
- `web.directory.max-requests`: directory探索の自動実行上限
- `web.file.max-requests`: extension付きfile探索の自動実行上限
- `web.vhost.max-requests`: vhost探索の自動実行上限
- `web.vhost.calibration-samples`: vhost校正sample数
- `web.vhost.calibration-confidence`: 自動filter採用の最低信頼度
- `password.max-requests`: password/username探索の自動試行上限
- `dns.max-queries`: DNS subdomain探索の自動query上限

## 保守コマンド

- `ctx status`: 現在のworkspace、path、database、Primary Targetを表示
- `ctx doctor`: 実行環境、設定、database、shell連携を診断
- `ctx reset [-y|--yes]`: ctxのdatabase、設定、管理データを削除。ワークスペースの一般ファイルやshell historyは削除しない
- `ctx -V|--version`: version表示
- `ctx -h|--help`: 簡易ヘルプ

## 保存データと安全上の注意

- 共有databaseは `${CTX_HOME:-$HOME/.ctx}/db.sqlite` にあります。
- workspaceごとの成果物とcacheは `${CTX_HOME:-$HOME/.ctx}/workspaces/<uuid>/` にあります。
- credential password、Cookie、token、command outputなどが平文で含まれる可能性があります。
- SQLiteを直接変更せず、登録コマンドまたはversion付きJSON APIを使用してください。
- `reset`、workspace/project削除、Web/cache clear、credential削除は対象を確認してから実行してください。

## よくある問題

- `ctx workspace not found`: 対象ディレクトリで `ctx workspace init` を実行します。
- `no primary target`: `ctx target add <ip>` または `ctx target use <name>` を実行します。
- serviceがない: `ctx scan` を実行するか、アドオン側で明示的なURL/portを指定します。
- `/etc/hosts` を更新できない: `ctx hosts sync` を実行し、sudo再実行を許可します。
- JSON連携が失敗する: `ctx formats --format json --format-version 1.0` で対応機能を確認します。

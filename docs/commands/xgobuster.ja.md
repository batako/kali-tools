# xgobuster オンラインヘルプ

[English](./xgobuster.md)

`xgobuster` はGobusterをctxへ統合し、Web path/fileまたはDNS subdomainを列挙するCLIです。`xgo` は同じ実行ファイルへのshortcutです。

## 構文

```text
xgobuster [options] [gobuster-options]
xgobuster dns [options] [gobuster-options]
```

```sh
xgo
xgo --preset php
xgo -x php,txt
xgo dns --domain example.thm
```

引数なしはGobusterの `dir` 相当、`dns` を指定するとDNS modeです。ctx workspace、Primary Target、`ctx`、`gobuster` が必要です。

## Web探索

ctxのWeb service、hostname、Target IPからURLを決定してpathを列挙します。extensionまたはpresetを指定した検索はfile探索として扱い、それ以外はdirectory探索として扱います。

### Technology preset

`--preset` は想定技術に対応するextensionを設定し、file探索へ切り替えます。

| preset | Gobusterへ渡すextension |
| --- | --- |
| `php`, `wordpress` | `php,inc,phps` |
| `aspnet` | `asp,aspx,config` |
| `java` | `jsp,do,action` |
| `node` | `js,json` |
| `static` | `html,htm,js` |

`-x, --extensions <list>` を明示した場合は、その値をGobusterへ渡します。extensionなしの語もGobusterの通常動作に従って検索対象になります。

## DNS探索

```sh
xgobuster dns --domain target.thm
```

`-d, --domain` を省略した場合はctxのhostname情報からdomainを決定します。検出結果はctxへ保存され、後続のWeb探索でhostname候補として利用できます。

## Wordlistの自動選択

`-w` を省略するとctxへ推薦を問い合わせます。

- Web directory探索: `directory`
- extension付きfile探索: `directory`（同じbase wordをextensionごとに展開）
- DNS探索: `subdomain`

推薦priorityの高いwordlistから順に使用します。同一scopeで同じコマンドを再実行すると、既に試した語を除外し、上位から未実行の候補へ進みます。実行量は次のctx configで制限します。

```text
web.directory.max-requests
web.file.max-requests
dns.max-queries
```

`-w, --wordlist <path>` を指定した場合はそのfileだけを使用します。

## 対象の選択

- `-u, --url <url>`: 自動選択せずURLを指定します。
- `--host <hostname>`: 登録済みhostnameを使用します。
- `--ip`: Target IPをhostとして使用します。
- `--service <number>`: 検出済みWeb serviceを番号で選択します。
- `-d, --domain <domain>`: DNS modeのdomainを指定します。

`--url` は `--host`、`--ip`、`--service` と併用できません。`--host` と `--ip` も同時指定できません。

## Responseの除外

- `--exclude-status <code>`: status codeを除外します。複数値はGobusterが受け付ける形式で指定します。
- `--exclude-length <size>`: response body sizeを除外します。
- `-c, --cookies <value>`: Cookieを送信します。

wildcard responseがあるsiteでは、存在しないpathと同じstatus/sizeを除外してください。

## TLS

- `-k, --no-tls-validation`: TLS証明書検証を無効化します。
- `--tls-verify`: TLS証明書を検証します。

両者は併用できません。

## 状態とcache

```sh
xgo --status
xgo --preset php --status
xgo dns --domain target.thm --status
xgo --clear-cache
```

状態とcacheはmode、Target/URL/domain、preset、extensionなどを含むscopeごとに分かれます。`--clear-cache` はそのscopeのwordlist進捗を削除し、Gobusterは実行しません。保存済みWeb discoveryも含めて消す場合は `ctx web clear` を使用します。

## 保存される情報

展開したGobuster command、終了状態、stdout/stderr、使用wordlistと試行語、検出したpath/file/subdomainをctxへ保存します。Web結果は `ctx web ls`、実行履歴は `ctx log` で確認できます。

## よくある問題

- 次のwordlistへ進まない: 同一scopeか、request上限へ達していないかを `--status` で確認します。
- extensionを含むfile探索をしたい: `--preset` または `-x` を指定します。
- wildcard errorになる: `--exclude-status` または `--exclude-length` を指定します。
- hostnameで接続できない: `ctx host ls` と `ctx hosts sync` を確認します。
- wordlistがない: `ctx wordlist --kind directory --usable-only` などで環境を確認します。

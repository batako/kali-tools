# xffuf オンラインヘルプ

[English](./xffuf.md)

`xffuf` はffufをctxのTarget、Web service、wordlist推薦、探索履歴へ統合するラッパーです。virtual hostとquery parameterの列挙に用途を限定し、対象とwordlistを可能な限り自動決定します。

## 構文

```text
xffuf <vhost|param> [options] [ffuf-options]
```

```sh
xffuf vhost --domain example.thm
xffuf param -u 'http://example.thm/?FUZZ=value'
xffuf param -u 'http://example.thm/?page=FUZZ' -mc 200,302
```

ctx workspaceとPrimary Targetが必要です。`ffuf` と `ctx` はKali service内にインストールされている必要があります。

## Mode

### `vhost`

HTTPの `Host` headerへwordlistの語を組み込み、virtual hostを列挙します。domainを明示しない場合はctxに登録されたhostnameなどから候補を決め、HTTP serviceと接続先IPもctxから選択します。

明示的なmatcher/filterがない場合は、存在しないhostへのresponseをsample採取してstatus、size、word数などを校正し、誤検出を減らします。校正を無効化するには `--no-auto-filter` を使います。

### `param`

URL query内の `FUZZ` の位置で用途を判定します。URLには `FUZZ` をちょうど1個含めてください。

```text
?FUZZ=value   parameter名を列挙する（parameter-name）
?name=FUZZ    parameter値を列挙する（parameter-value）
```

用途ごとに別のctx wordlist kindを使用します。`?FUZZ=https://example.com` はredirectを起こすparameter名の候補を探す形ですが、redirectだけに絞るにはffufのmatcherを明示します。

```sh
xffuf param -u 'http://target.thm/?FUZZ=https://example.com' -mc 301-308
```

`-mc` は指定responseだけを採用し、`-fc` は指定responseを除外します。したがって `-fc 301-308` はredirectを探す指定ではありません。

## Wordlistの選択と継続

`-w` を省略すると、ctxへ次のkindを問い合わせ、その推薦順で利用します。

- `vhost`: `subdomain`
- parameter名: `parameter-name`
- parameter値: `parameter-value`

同じTarget、URL、modeなどからなる検索scopeでは、成功済みwordlistと試行済みwordを記録します。同じコマンドを再実行すると、推薦順の上位から未試行分を選びます。手動の `--next` はありません。

`-w, --wordlist <path>` は任意のwordlistを明示します。この場合、自動推薦は使用しません。

## 対象の選択

- `-u, --url <url>`: ctx serviceの代わりにURLを明示します。
- `--host <hostname>`: 登録済みxhost hostnameを使用します。
- `--ip`: hostnameではなくTarget IPをHTTP hostとして使用します。
- `--service <number>`: 表示されたWeb service番号を選択します。
- `-d, --domain <domain>`: vhostの基準domainを明示します。`param` では使用できません。

`--url` と `--host` / `--ip`、`--url` と `--service` は併用できません。

## RequestとTLS

- `-c, --cookies <value>`: Cookie headerを指定します。
- `-k, --no-tls-validation`: TLS証明書を検証しません。
- `--tls-verify`: TLS証明書を検証します。

自己署名証明書を使う演習環境以外では、検証を無効化しないでください。

## MatchとFilter

代表的なffuf optionをそのまま渡せます。

```text
-mc  match status       -fc  filter status
-ml  match lines        -fl  filter lines
-mr  match regex        -fr  filter regex
-ms  match size         -fs  filter size
-mw  match words        -fw  filter words
```

matchは「残したいresponse」、filterは「捨てたいresponse」の指定です。手動filterを指定すると自動校正は行いません。

## 校正と試行

- `--suggest`: 校正結果を表示し、確認後にtrialを実行できます。手動filterとは併用できません。
- `--trial`: ctx log、cache、host登録を行わず試します。
- `--no-auto-filter`: vhostの自動校正を無効にします。

`--trial` は実行結果を次回の進捗へ反映しません。

## 状態と削除

```sh
xffuf vhost --status
xffuf vhost --clear-cache
```

`--status` はscope内のwordlist進捗を表示します。`--clear-cache` は該当する検索進捗を削除します。Web discovery自体をまとめて消す場合は `ctx web clear` を使用します。

## 保存される情報

通常実行では、展開後ffuf command、終了状態、stdout/stderr、wordlist進捗、検出したhostまたはparameterをctxへ保存します。passwordやsession Cookieをoptionへ書くとlogに残る可能性があります。

## よくある問題

- `no ... wordlist found`: `ctx wordlist --kind <kind> --usable-only` で候補を確認し、必要なら `-w` で明示します。
- redirectだけを探せない: `-fc` ではなく `-mc 301-308` を使用します。
- 結果が大量に出る: vhostでは自動校正を有効にするか、`-fs` などを指定します。
- 同じwordlistを最初から実行したい: 対象scopeを確認して `--clear-cache` を使用します。
- URLの `&` がshellで解釈される: URL全体をsingle quoteで囲みます。

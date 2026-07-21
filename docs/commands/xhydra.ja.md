# xhydra オンラインヘルプ

[English](./xhydra.md)

`xhydra` はHydraをctxのTarget、service、credential、wordlist推薦、試行履歴へ統合するCLIです。対応modeはHTTP form、SSH、FTP、SMBです。許可された演習環境だけで使用してください。

## 構文

```text
xhydra <http|ssh|ftp|smb> [options]
```

```sh
xhydra ssh -u testuser
xhydra ftp -u anonymous
xhydra smb -u smbuser -t 1
xhydra http -u admin --request login.req --fail-body 'Invalid password'
```

## Password探索とUsername探索

### Password探索

`-u, --username` を指定し、password wordlistを順に試します。

```sh
xhydra ssh -u root
```

### Username探索

固定passwordを `--password` で指定し、`-u` を省略するとusername wordlistを試します。

```sh
xhydra ssh --password password
xhydra ssh --password password -L users.txt
```

`-L, --user-list` はusername wordlistを明示します。password listの明示には `-P, --password-list` を使います。

## Wordlistと再開

自動選択時はctxの `password` または `username` 推薦をpriority順に利用します。同じmode、host、port、usernameなどからなるscopeでは試行済みの語を保存し、再実行時は未試行分へ進みます。別usernameを指定した場合は別scopeとなり、そのusernameでは先頭wordlistから開始します。

自動試行数の上限は次で設定します。

```sh
ctx config get password.max-requests
ctx config set password.max-requests 5000
```

## SSH、FTP、SMB

- `--host <host>`: Primary Target以外を明示します。
- `-p, --port <port>`: portを明示します。
- `--service <number>`: ctxの検出serviceを番号で選択します。
- `-t, --tasks <number>`: Hydraの並列task数を上書きします。

portの既定値はSSH 22、FTP 21、SMB 445です。ctxに該当serviceが1件あれば自動選択し、複数なら選択を求めます。

SSH、FTP、SMBのtask既定値は現在すべて4です。`-t` で1以上の値へ変更できます。対象serviceが接続数を厳しく制限する場合は1まで下げてください。接続拒否が続く場合は並列数だけでなく、serviceの稼働、protocol、port、接続元制限を確認します。

SMB modeはHydraの `smb2` moduleを使用します。share名（`public`、`private` など）はpassword認証先ではないため選択しません。share列挙・接続は `xsmb`、SMB protocolの詳細な認証確認には必要に応じてNetExecなどを使用してください。

## HTTP form

HTTP modeはPOST formを対象とします。raw requestまたはURL/bodyからrequest templateを作ります。

```sh
xhydra http -u admin -r login.req --fail-body 'Invalid credentials'
xhydra http -u admin --url http://target/login \
  --data 'username=^USER^&password=^PASS^' \
  --fail-status 401
```

- `-r, --request <file>`: `req` と同形式のraw HTTP requestを読み込みます。
- `--url <url>`: request fileを使わずURLを指定します。
- `--data <body>`: POST bodyを指定します。
- `--user-field <name>` / `--password-field <name>`: form field名を指定します。body内の `^USER^` / `^PASS^` も使用できます。

`--request` と `--url` / `--data` は併用できません。

### 成否判定

少なくとも対象に合った判定条件を使用してください。

- `--fail-json <field=value>` / `--success-json <field=value>`
- `--fail-body <text>` / `--success-body <text>`
- `--fail-status <code>`
- `--success-redirect`: HTTP 302を成功とみなします。

誤った判定はfalse positiveまたはpassword見落としの原因になります。実際の失敗responseを `req` やbrowserで先に確認してください。

## 状態とcache

```sh
xhydra ssh -u root --status
xhydra ssh -u root --clear-cache
```

`--status` と `--clear-cache` はpassword探索ではusernameが必要です。削除scopeにはmode、username、host、portなどが含まれるため、FTPのcache削除はSSHや別usernameのcacheを削除しません。

## Credential保存とlog

Hydraが有効な組み合わせを発見すると、対応scopeのcredentialとしてctxへ保存します。command、終了状態、stdout/stderr、wordlist進捗はctx logへ保存します。passwordそのものをcommand lineへ指定するとprocess listやlogに現れる可能性があるため注意してください。

## よくある問題

- SSHのparallel warning: 既定は `-t 4` です。対象が不安定ならさらに下げます。
- `Connection refused`: `nc -vz <host> <port>`、ctx service、protocolを確認します。portが開いていてHydraだけ失敗する場合はmodule互換性も疑います。
- passwordを見つけてもloginできない: Hydraの成否条件、account制限、認証方式を確認します。
- 最初から再試行したい: 正しいscopeを指定して `--clear-cache` を実行します。
- wordlist候補を確認したい: `ctx wordlist --kind password --usable-only` を使用します。

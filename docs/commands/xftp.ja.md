# xftp オンラインヘルプ

[English](./xftp.md)

`xftp` はctxのPrimary Targetへlftpで接続するCLIです。scope `ftp` のcredentialと検出FTP serviceを利用し、接続履歴をctxへ保存します。

## 構文

```text
xftp [credential-id|username]
```

```sh
xftp
xftp anonymous
xftp 7
```

## TargetとPort

TargetはctxのPrimary Target IPです。FTP serviceがない場合はport 21、1件ならそのport、複数なら対話選択します。現行xftpにはhost/port optionはありません。

## Credential

- credentialが0件で引数なし: anonymous接続を試します。
- 1件: 自動選択します。
- 複数: 対話選択し、前回成功したIDを既定候補にします。
- 数値引数: credential IDを選びます。
- 文字列引数: usernameで選びます。未登録ならpasswordなしで接続します。

password付きcredentialは `LFTP_PASSWORD` environment variableでlftpへ渡します。ctx logのcommandにはpasswordを含めません。

接続時は不要なretryを繰り返さないよう `net:max-retries 0` を設定します。credential使用時はNOOPを送り、初期認証失敗を正常終了として扱わないようにします。

## Interactive操作

接続後はlftpのcommandを使用します。

```text
ls                 remote file一覧
pwd                remote current directory
get <file>         download
put <file>         upload
mirror <dir>       directory取得
exit               終了
```

転送先と上書きはlftpの指定に従います。

## Log

username、Target、port、終了状態、stdout/stderrをctx logへ保存します。passwordは保存しません。正常終了したcredential IDは次回の既定候補になります。

## 必要なcommand

- `ctx`
- `lftp`

## よくある問題

- anonymous loginできない: FTP credentialをctxへ登録してusernameまたはIDを指定します。
- 別portを選べない: `ctx scan` でserviceを登録します。直接指定が必要ならlftpを使用します。
- 接続がretryされ続ける: xftpはretry 0を設定しますが、lftp内部処理やinteractive commandの設定も確認します。
- passwordがpromptされる: passwordなしcredentialを選んでいる可能性があります。

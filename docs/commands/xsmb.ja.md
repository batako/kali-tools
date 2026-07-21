# xsmb オンラインヘルプ

[English](./xsmb.md)

`xsmb` はctxのPrimary Targetで利用可能なSMB shareを列挙し、選択したshareへsmbclientで接続するCLIです。

## 構文

```text
xsmb [credential-id|username]
```

```sh
xsmb
xsmb smbuser
xsmb 9
```

## 処理の流れ

1. ctxからPrimary TargetとSMB serviceを取得します。
2. anonymousの `smbclient -L` でDisk shareを列挙します。
3. `IPC$` を除外し、複数なら接続先shareを選択させます。
4. 指定credentialまたはanonymousでinteractive smbclientを起動します。
5. 接続結果をctx logへ保存します。

share列挙は現在anonymousで行います。anonymous列挙を禁止するserverでは、credentialが正しくてもshare候補を取得できず終了することがあります。

## TargetとPort

TargetはctxのPrimary Target IPです。SMB serviceがなければ445、1件ならそのport、複数なら対話選択します。service名 `smb`、`microsoft-ds` などが候補になります。現行xsmbにhost/port optionはありません。

## Credential

scope `smb` のcredentialを使用します。選択規則はxssh/xftpと同様です。

- 0件で引数なし: anonymous（`-N`）
- 1件: 自動選択
- 複数: 対話選択
- 数値: credential ID
- 文字列: username。未登録ならpassword promptをsmbclientへ任せます。

保存passwordは `PASSWD` environment variableで渡し、ctx logへ含めません。

## Interactive操作

接続後はsmbclientのcommandを使用します。

```text
ls                 remote file一覧
cd <dir>           remote directory移動
get <file>         download
put <file>         upload
recurse ON         再帰操作を有効化
prompt OFF         個別確認を無効化
exit               終了
```

## xhydra smbとの違い

`xhydra smb` はusername/passwordの組み合わせをSMB2 authenticationへ試すためのものです。`xsmb` はshareを列挙・選択してfile操作するためのものです。`public` や `private` はshare名であり、Hydraの解析対象を指定する値ではありません。

## Log

選択share、username、Target、port、終了状態、stdout/stderrをctx logへ保存します。share列挙の内部commandは接続logとは別に詳細保存されません。正常終了したcredentialは次回の既定候補になります。

## 必要なcommand

- `ctx`
- `smbclient`

## よくある問題

- `no SMB shares found`: anonymous share列挙が禁止されているか、Disk shareがありません。`smbclient -L //<IP> -U <user>` で確認します。
- 445はopenだが接続できない: dialect、authentication方式、server policyを確認します。
- shareを直接指定したい: 現行xsmbは列挙後の選択のみです。直接指定にはsmbclientを使用します。
- passwordがlogに見える: xlogの展開commandにはusernameだけを残す仕様です。別途debug outputを共有する際も秘密情報を確認してください。

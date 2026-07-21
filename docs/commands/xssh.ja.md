# xssh オンラインヘルプ

[English](./xssh.md)

`xssh` はctxのPrimary TargetへSSH接続するCLIです。保存済みSSH credentialと検出serviceを選び、接続履歴をctxへ記録します。

## 構文

```text
xssh [credential-id|username|key]
```

```sh
xssh
xssh testuser
xssh 12
xssh key
```

## 接続先

接続hostは現在のctx workspaceのPrimary Target IPです。SSH portは次の順で決定します。

1. ctxにSSH serviceがない場合は22
2. 1件だけある場合はそのport
3. 複数ある場合は対話選択

`xssh` 自体にhost/port optionはありません。対象を変える場合は `ctx target use`、service情報を更新する場合は `ctx scan` を使用します。

## Credentialの選択

ctxのscope `ssh` に保存されたcredentialを利用します。

- 引数なしで0件: usernameなしで通常のSSHを起動します。
- 引数なしで1件: そのcredentialを自動選択します。
- 引数なしで複数: 一覧から選択します。前回成功したcredentialが既定候補です。
- 数値: credential IDを指定します。
- 文字列: usernameで検索します。同名が複数なら選択します。
- 未登録username: passwordなしのusernameとして接続します。

```sh
ctx credential set ssh testuser 'password'
xssh testuser
```

password付きcredentialでは `sshpass` を使い、passwordは `SSHPASS` environment variableで渡します。展開後commandとctx logにpasswordは含めません。

## Host key確認

SSHには `StrictHostKeyChecking=accept-new` を設定します。未登録host keyは自動追加し、既知hostのkeyが変化した場合は通常のSSHと同様に拒否します。初回接続で手動の `yes` を事前入力する必要はありません。

## `xssh key`

localの `~/.ssh/id_ed25519.pub` を対象へ登録するためのshell commandを表示します。keyが存在しない場合は `ssh-keygen -t ed25519` を起動します。

表示されたcommandは対象host上で実行し、`~/.ssh/authorized_keys` を安全なpermissionで作成して、同じpublic keyの重複登録を避けます。`xssh key` 自身はremote hostへ接続せず、keyを自動送信もしません。

## Logと状態

接続開始・終了、username、host、port、終了codeをctx logへ保存します。interactive sessionの標準入出力は保存しません。TTY上で入力したcommandやpasswordをctxが記録する機能ではありません。

保存済みcredentialで正常終了した場合、そのIDを次回の対話選択で既定候補にします。

## 必要なcommand

- `ctx`
- `ssh`
- password付きcredentialの場合は `sshpass`

## よくある問題

- `no active workspace`: `ctx workspace init` またはworkspace配下へ移動します。
- `no primary target`: `ctx target add` / `ctx target use` を実行します。
- host key changed: 対象IPの再利用か攻撃の可能性を確認してからknown_hostsを修正します。
- 通常のsshでは接続できるがxsshでは失敗する: ctxのPrimary Target、選択username、検出portを確認します。
- 別portを直接指定したい: 現行xsshにはoptionがないため、ctx serviceへ登録されるようscanするか通常の `ssh -p` を使用します。

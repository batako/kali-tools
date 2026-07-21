# xscp オンラインヘルプ

[English](./xscp.md)

`xscp` はctxのPrimary Targetとの間でfileを転送するSCP wrapperです。SSH credentialとservice選択はxsshと同じ考え方で、転送履歴をctxへ保存します。

## 構文

```text
xscp <upload|download> <source> [destination] [credential-id|username] [options]
```

```sh
xscp upload shell.php /tmp/shell.php testuser
xscp download /var/www/html/config.php ./config.php testuser
xscp upload report.txt --port 2222
```

## Upload

```text
xscp upload <local-source> [remote-destination] [credential]
```

local fileをTargetへ送ります。destination省略時はsourceのbasenameをremote pathとして使います。

```sh
xscp upload notes.txt
# scp notes.txt user@target:notes.txt
```

## Download

```text
xscp download <remote-source> [local-destination] [credential]
```

Target上のfileをlocalへ取得します。destination省略時はsourceのbasenameをlocal pathとして使います。

## Credential

scope `ssh` のctx credentialを利用します。数値はcredential ID、文字列はusernameです。省略時は0件ならusernameなし、1件なら自動選択、複数なら対話選択します。未登録usernameもpasswordなしで使用できます。

password付きcredentialでは `sshpass -e` を使い、passwordはenvironment variableで渡します。logへpasswordを埋め込みません。

## PortとService

- `-p, --port <port>`: SSH portを直接指定します。
- `--service <number>`: ctxのSSH service一覧から番号を指定します。

両者は併用できません。どちらも省略した場合、SSH serviceが0件なら22、1件なら自動、複数なら対話選択です。

## Pathの解釈

sourceとdestinationはscpへ単一pathとして渡します。space、colon、shell metacharacterを含むpathはlocal shellとscp remote pathの双方の解釈に注意してください。directoryの再帰転送optionは現行xscpでは提供していません。

既存destinationの上書き確認はxscp独自には行わず、scpの動作に従います。重要fileを上書きしないようdestinationを明示してください。

## Log

direction、local/remote path、username、Target IP、port、終了状態、stdout/stderrをctx logへ保存します。正常終了した保存済みcredentialは次回選択の既定候補になります。

## 必要なcommand

- `ctx`
- `scp`
- password付きcredentialの場合は `sshpass`

## よくある問題

- positional argumentが曖昧: credentialを指定する場合はdestinationも明示し、3番目のpositionへcredentialを置きます。
- permission denied: remoteのusername、destination permission、credentialを確認します。
- local download先を意図せず上書きした: xscpにbackup機能はありません。destinationを明示します。
- 複数SSH portがある: 対話選択、`--service`、または `--port` を使用します。

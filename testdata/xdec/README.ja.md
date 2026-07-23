# xdec 動作確認データ

すべての hash は平文 `password` 用です。テスト専用の値です。

## 即時デコード

```sh
xdec -f base64.txt
# password

xdec -f hex.txt
# password
```

## hash 判定・確認プロンプト

```sh
xdec -f md5.txt
xdec -f sha1.txt
xdec -f sha256.txt
xdec -f ntlm.txt
xdec -f bcrypt.txt
```

通常は高コスト処理の確認が表示されます。`-w` なしでは ctx が推薦する利用可能な password wordlist を順番に試します。テスト用 wordlist を明示して実行する場合は、次のようにします。

```sh
xdec -w wordlist.txt --yes -f md5.txt
```

## ユーザー名付き hash

```sh
xdec --scope ssh --yes -w wordlist.txt -f user-hashes.txt
```

ctx workspace と primary target が設定され、`--scope ssh` を指定している場合、成功した credential が自動登録されます。

```sh
xcredential ls ssh
xlog
```

## 複数行の取得結果

```sh
cat command-output.txt | xdec --dry-run
cat command-output.txt | xdec --scope ssh --yes -w wordlist.txt
```

## 複数 wordlist の親子ログ

```sh
xdec --yes \
  -w wordlist-empty.txt \
  -w wordlist.txt \
  -f user-hashes.txt
```

`xlog` で、xdec の親ログの下に wordlist ごとの子 step が作成されていることを確認します。

## 非対話 stdin

```sh
cat md5.txt | xdec -w wordlist.txt
# 非対話なので expensive operation は実行せず、--yes が必要

cat md5.txt | xdec --yes -w wordlist.txt

# 既存 state を破棄して、この入力だけ再解析
xdec --refresh --yes -w wordlist.txt -f md5.txt
```

## SSH 秘密鍵

`id_rsa` などは、暗号化の有無を自動判定して直接渡せます。未暗号化鍵は解析せず、暗号化鍵だけ `ssh2john` 経由で解析します。

```sh
xdec --yes -w wordlist.txt -f ~/.ssh/id_rsa
```

fixture の暗号化鍵のパスフレーズは `password` です。未暗号化鍵では John を実行せず、パスワード不要と表示されます。

```sh
xdec decode --yes -w wordlist.txt -f id_ed25519_encrypted
xdec decode -f id_ed25519_unencrypted
xdec decode --yes -w wordlist.txt -f id_rsa_encrypted
xdec decode -f id_rsa_unencrypted
```

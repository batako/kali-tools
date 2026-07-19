# ctx登録コマンド

この文書は、カスタムコマンドと公式コマンドがctxの共有データを登録するために利用できる、既存のコマンド仕様を定義します。JSON APIではなく、CLIから利用する登録仕様です。

連携方法の選択は[ctx連携ガイド](integration.ja.md)、構造化出力が必要な場合は[ctx JSON API](api.ja.md)を参照してください。

## 仕様の範囲

この文書に記載したコマンド形式では、次に依存できます。

- コマンド名とオプション名
- 引数の順序と意味
- 記載された重複時の動作
- 成功時の終了コード`0`
- 入力不正、コンテキスト不足、競合、保存失敗時の非0終了コード

現在の通常コマンドの失敗コードは`1`です。JSON endpointとは異なり、引数不正を終了コード`2`へ分けず、機械可読なerror codeも返しません。

次には依存できません。

- stdoutとstderrの正確な文言
- 空白、句読点、色、行の順序
- 人間向け成功メッセージに含まれるID
- 削除・再作成後も内部DB IDが同じであること

成功時は人間向けの確認をstdout、失敗時は人間向けの診断をstderrへ出力します。どちらも利用者向けメッセージとして扱い、データとして解析しません。

## 前提条件

すべての登録コマンドには、有効なworkspaceが必要です。

```sh
ctx workspace init
```

Target、host、credentialの登録には、後述するtarget contextも必要です。Noteは有効なworkspaceだけで登録できます。

## Target context

Targetは、host、service、credential、target単位の調査結果を関連付ける同一性を定義します。

### Primary targetの設定・更新

```sh
ctx target set <ip>
ctx target update <ip>
ctx target <ip>
```

現在、`set`、`update`、短縮形式は同じ処理を行います。

- IPは有効なIPv4またはIPv6でなければならない
- targetが存在しない場合は`default`というprimary targetを作成する
- primary targetが存在する場合は、targetの同一性を置き換えずにIPを更新する
- IP変更後も、そのtargetに関連するレコードは同じtargetへ関連付いたまま維持する

同じIPでもう一度実行した場合も成功します。

### 別targetの追加

```sh
ctx target add <ip> [--name <name>]
```

- IPは有効でなければならない
- nameを省略した場合はctxが生成する
- workspace内の最初のtargetはprimaryになる
- target nameが競合した場合は、別targetの選択や置換を行わず失敗する

既存targetをprimaryにする場合:

```sh
ctx target use <name>
```

Target削除は破壊的処理であり、登録操作には含めません。

## Host

Primary targetへホスト名を登録します。

```sh
ctx host add <hostname>
ctx host <hostname>
```

名前を指定したtargetへ登録します。

```sh
ctx host add <hostname> --target <target-name>
```

動作:

- 有効なworkspaceが必要
- `--target`を省略した場合はprimary targetが必要
- `--target`を指定した場合は、そのtargetが存在しなければならない
- 前後の空白と末尾の`.`を1つ取り除く
- 小文字で保存する
- labelは空ではなく63文字以内
- hostname全体は253文字以内
- 英数字、hyphen、underscoreを使用可能
- labelの先頭と末尾にhyphenは使用不可
- 空白、`/`、`\`、`:`は使用不可

一意性はtargetと正規化済みhostnameの組み合わせです。同じtargetへ同じhostnameを再登録しても成功し、manual登録としてレコードを更新します。同じhostnameを別targetへ関連付けることは可能です。

## Credential

Credentialの同一性は、現在のprimary target、`scope`、`username`の組み合わせです。

### 作成または置換

```sh
ctx credential set <scope> <username> [password]
ctx credential <scope> <username> [password]
```

`set`と短縮形式は、credentialがなければ作成し、存在する場合はpasswordを置き換えます。

### 新規作成のみ

```sh
ctx credential add <scope> <username> [password]
```

同じtarget、scope、usernameが既に存在する場合、`add`は失敗します。繰り返し可能な作成・置換には`set`を使用します。

### 既存更新のみ

```sh
ctx credential update <scope> <username> [password]
```

対象credentialが存在しない場合、`update`は失敗します。

3つの形式に共通する動作:

- 有効なworkspaceとprimary targetが必要
- scopeとusernameには、空白以外の文字が1文字以上必要
- scopeとusernameは指定されたまま保存し、大文字・小文字を正規化しない
- passwordを省略すると未設定として保存するため、`set`または`update`で既存passwordを削除する

### 秘密情報の扱い

現在のコマンドはpasswordを引数でのみ受け取ります。そのため、shell history、process list、terminal recording、人間向け成功出力へ露出する可能性があります。

その露出を許容できない環境では、無人連携から秘密情報を渡す目的に使用しないでください。成功出力も解析・保存しないでください。標準入力からJSONを安全に受け取る登録形式は現時点では未実装であり、具体的な連携で必要になった場合だけ設計します。

## Note

有効なworkspaceのtimelineへnoteを追加します。

```sh
ctx note <text...>
```

動作:

- 残りの引数をsingle spaceで結合する
- 前後の空白を取り除く
- 空のnoteは失敗する
- target単位ではなくworkspace単位で保存する
- 同じ本文を複数回登録でき、それぞれ別のtimeline entryになる

正確な空白、改行、任意のbinary contentを保持する必要がある場合、`ctx note`は適切な機械連携ではありません。

## 未対応の登録対象

現在、次を登録する汎用公開コマンドはありません。

- serviceとscan record
- Web探索結果
- Web wordlist実行履歴
- 任意のDB record
- ツール専用の進捗cache

これらの内部Go関数とDBテーブルは、外部向けの登録仕様ではありません。具体的な用途から入力検証付きctxコマンドまたは機械入力が必要だと確認できるまで、未対応の結果は生成したツール側で管理します。

## 呼び出し側の規則

継続的に保守する連携では、次を守ります。

1. インストール済みctxをshellを経由せず実行する
2. 各引数を分離して渡す
3. 終了コード`0`を成功、それ以外を失敗として扱う
4. stderrは利用者へ表示するが、文言を条件判定に使わない
5. 人間向けstdoutからIDや値を取得しない
6. 露出を許容できない秘密情報をargvへ渡さない

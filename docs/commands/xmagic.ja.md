# xmagic オンラインヘルプ

[English](./xmagic.md)

`xmagic` は入力fileの先頭magic numberを別形式へ置き換えたcopyを作るCLIです。source fileは変更せず、出力名を自動決定します。file upload検証など、許可された演習環境で使用してください。

## 構文

```text
xmagic [ls]
xmagic set <type> <file>
```

```sh
xmagic
xmagic set jpg image.png
xmagic set gif shell.php
```

引数なしは `xmagic ls` と同じです。

## 対応type

| type | 書き込むsignature | alias |
| --- | --- | --- |
| `gif` | `GIF89a` | `gif89a` |
| `jpg` | `FF D8 FF` | `jpeg` |
| `png` | `89 50 4E 47 0D 0A 1A 0A` | なし |
| `pdf` | `%PDF-` | なし |
| `zip` | `50 4B 03 04` | なし |

GIF87aはsource signatureとして認識しますが、出力typeには指定できません。

## ReplaceとPrepend

source先頭が既知のsignatureなら、そのsignature長のbyte列を取り除き、指定typeのsignatureへ置き換えます。

```text
PNG source + set jpg
=> PNG 8-byte signatureを除去し、JPEG 3-byte signatureを追加
```

source先頭が既知でなければ、既存内容を削らず指定signatureを先頭へ追加します。

```text
PHP source + set gif
=> GIF89a + 元のPHP全体
```

これはfile全体を正しい画像やarchiveへ変換する処理ではありません。先頭signatureだけを操作するため、`file`、MIME判定、parserごとに結果が異なります。

指定typeのmagic numberを既に持つfileはerrorにします。

## 出力名

sourceと同じdirectoryへ、元のstemとextensionの間にtarget typeを入れます。

```text
image.png     -> image.jpg.png
shell.php     -> shell.gif.php
```

既に存在する場合は `.2`、`.3` のように連番を加えます。

```text
image.jpg.png
image.jpg.2.png
```

既存fileは上書きしません。temporary fileへ書き込み、完了後に新規pathとして公開します。permissionはsourceのpermission bitを引き継ぎます。

## 完了時の表示

次を表示します。

- replaceまたはprependしたtype
- 作成した絶対path
- OSが検出したcontent type
- sourceとoutputのSHA-256
- sourceが変更されていないこと

操作情報としてsource/output path、追加・除去byte、両SHA-256、日時も内部状態へ保存します。現行versionにはmagic numberを元へ戻す公開commandはありません。

## 安全性と制約

- regular fileだけを入力できます。
- sourceはread-onlyで開き、直接変更しません。
- outputが既にある場合は別名を選びます。
- signatureが正しくてもfile構造全体がその形式として有効とは限りません。
- 悪意あるfileの作成・配布ではなく、管理下のupload validation検証などに限定してください。

## よくある問題

- `file already has ...`: 同じtypeへの変更は不要です。
- upload先で拒否される: extension、MIME、画像decode、再encodeなどmagic number以外の検証が行われています。
- 出力を元へ戻したい: 現行xmagicにrestore commandはないためsource fileを使用します。
- 想定外のtypeと判定される: parserがsignature以外も検証しているため、`file` やhex dumpで確認します。

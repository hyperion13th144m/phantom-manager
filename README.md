# PhantomManager

PhantomManager は、`phantom-release` の全文検索システムを Windows 上で管理するための WinForms アプリです。

## 必要なツール

- Windows 10 / 11
- .NET SDK 8.0 以降
- Docker Desktop for Windows
- Git for Windows

確認例:

```powershell
dotnet --info
docker --version
git --version
```

Docker Desktop は起動した状態で使います。

## ディレクトリ構成

想定する配置は次の通りです。

```text
app/
  phantom-manager.exe
  phantom-release/
PhantomManager/
  PhantomManager.csproj
  Form1.cs
  ...
```

`phantom-release` はアプリの `clone` ボタンから取得できます。

## ビルド

開発用ビルド:

```powershell
dotnet build
```

配布用 exe の作成:

```powershell
dotnet publish -c Release -r win-x64 --self-contained true -p:PublishSingleFile=true -p:AssemblyName=phantom-manager -p:EnableCompressionInSingleFile=true -o ..\app
```

成果物は `..\app\phantom-manager.exe` に作成されます。

## 簡単な使い方

1. `phantom-manager.exe` を起動します。
2. Docker Desktop for Windows と Git for Windows の状態を確認します。
3. `phantom-release` が無い場合は `clone` を押します。
4. データディレクトリを選択し、`.env 保存` を押します。
5. `fetch / タグ取得` でバージョン一覧を取得します。
6. バージョンを選び、`チェックアウト` を押します。
7. `起動` で `docker compose up -d` を実行します。
8. サービス起動後、表示された `http://<IP address>:8080/` を開きます。
9. 停止するときは `停止` を押します。

サービスが起動中の間は、`.env 保存` やタグ切り替えなど、実行中サービスと競合しやすい操作は無効になります。

## ミラーバッチ

`元データ選択` で元データのディレクトリを選び、`ミラーバッチ作成` を押すと、次のファイルを作成します。

```text
app\bat\mirror.bat
```

このバッチは `robocopy` で元データをデータディレクトリへミラーします。ログは `app\log\mirror.log` に出力されます。

## データベース初期化

サービス起動中に `データベース初期化` を押すと、Elasticsearch のインデックスを削除して作り直し、`phantom-release\es\mapping.json` をアップロードします。

この操作は既存インデックスを削除するため、実行前に確認ダイアログが表示されます。

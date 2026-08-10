# phantom-manager

[phantom-release](https://github.com/hyperion13th144m/phantom-release) の全文検索システムを
Windows + WSL2 上で運用するためのユーティリティです。

**WSL2 の Ubuntu 内で動くローカル Web サーバ**として起動し、Windows 側のブラウザから操作します。
WSL2 の localhost forwarding により、WSL 内の `127.0.0.1:7777` は Windows の `localhost:7777` から見えます。

> 旧バージョンは Windows ネイティブの WinForms アプリで、`wsl.exe` 経由で WSL を外側から操縦していました。
> 新バージョンは逆に WSL の内側で動き、自分自身の環境を直接操作します。
> 旧実装は [`old/`](old/) に残してあります。

## できること

| 機能 | 内容 |
|---|---|
| リポジトリ管理 | phantom-release の clone / pull / バージョン（タグ）の固定と解除 |
| `.env.docker` 生成 | `.env.docker.sample` から WSL2 向けの設定を生成。既存の調整値は保持します |
| 取込スクリプト生成 | Windows のフォルダ（ネットワークドライブ可）から取込先へコピーする `.bat` を生成 |
| Docker Compose 操作 | build / pull / up -d / down と、サービス状態の一覧 |

実行したコマンドと出力はすべてログ枠に流れます。トラブル時はその内容を貼れば原因が追えます。

## 必要な環境

- Windows 10 / 11 + WSL2（Ubuntu）
- Docker Desktop for Windows
  - **Settings → Resources → WSL Integration で対象のディストリを有効にしてください。**
    無効のままだと WSL 内の `docker` は使えません。manager の環境チェックがこの状態を検出して手順を表示します。
- Git（WSL 内、`sudo apt install git`）

## インストール

WSL の Ubuntu 内で実行します。ランタイムの導入は不要です。

```bash
curl -fsSL -o phantom-manager https://github.com/hyperion13th144m/phantom-manager/releases/latest/download/phantom-manager && chmod +x phantom-manager
```

## 起動

```bash
./phantom-manager
```

`http://localhost:7777` を Windows の既定ブラウザで開きます。自動で開かない場合は URL を手で開いてください
（`/etc/wsl.conf` で interop を無効にしている環境では自動起動しません）。

| オプション | 既定値 | 説明 |
|---|---|---|
| `-port` | `7777` | listen ポート。使用中なら次の空きポートを使います |
| `-release` | `~/phantom/phantom-release` | phantom-release の配置先 |
| `-no-open` | | ブラウザを自動で開かない |
| `-version` | | バージョンを表示して終了 |

listen は `127.0.0.1` に固定しています。認証のない操作画面を LAN に晒さないためです。

## 使い方

1. **環境チェック**で Docker / Git / phantom-release の状態を確認します
2. **クローン**で phantom-release を取得します（既定の配置先は `~/phantom/phantom-release`）
3. **データディレクトリ**を設定して `.env.docker を保存`します
   - 既定は `~/phantom/data/src`（取込先）と `~/phantom/data`（展開先）です。
     コンテナは uid=gid=1000 で動きますが、WSL の既定ユーザーが uid 1000 なので `$HOME` 配下なら sudo は不要です
   - 他の PC から phantom を使う場合は `LAN IP を使う`で公開 URL を書き換えます
4. **取込スクリプトの作成**で取込元フォルダを選び、生成された `.bat` を Windows のエクスプローラから実行します
5. **ビルド** → **イメージを取得** → **起動**の順に実行します
6. サービス欄に表示される URL を開きます
7. 止めるときは**停止**を押します

サービス起動中は `.env.docker` の保存やバージョン切り替えができません。動いているコンテナがマウントしている
ディレクトリや compose ファイルを、その足元で書き換えないためです。無効なボタンにマウスを乗せると理由が出ます。

## 取込スクリプトについて

コピーは WSL 側ではなく **Windows 側の robocopy** が行います。`/mnt` 経由の I/O は 9p を通るため、
数万ファイル規模では実用的な速度が出ないためです。

コピー対象は次の 11 パターンです。前半 6 つはインターネット出願ソフトの電子データ、
後半 5 つは XML が残っておらず HTML と画像しかない文書（cendrillon が扱います）向けです。

```
*AAA.JWX  *AAA.JPC  *NNF.JWX  *NNF.JPC  *AFM.XML  *NFM.XML
*.HTM  *.HTML  *.GIF  *.JPG  *.PNG
```

生成される `.bat` は **Shift_JIS** で書き出します。cmd.exe はバッチファイルをコンソールのコードページで
読むため、UTF-8 で書くと日本語のパスが文字化けします。

## 開発

Go 1.25 以降が必要です。UI の HTML / CSS / JS は `embed` でバイナリに同梱されるので、ビルド手順は 1 つだけです。

```bash
make check   # gofmt + go vet + go test -race
make build   # ./phantom-manager を生成
make run     # 作業ツリーのまま起動
```

```text
main.go             起動・ポート確保・ブラウザ起動
internal/runner     コマンド実行と行単位のストリーミング
internal/jobs       単一ジョブの排他と SSE 配信
internal/wslenv     ディストリ名・パス変換・Windows 連携
internal/winfs      PowerShell 経由の Windows ファイルシステム参照
internal/envcheck   環境チェック
internal/gitrepo    phantom-release のリポジトリ操作
internal/envfile    .env.docker の生成
internal/mirror     取込スクリプトの生成
internal/compose    docker compose 操作
internal/server     HTTP ルーティングと操作可否の判定
web/                UI
old/                旧 WinForms 実装（参照用）
```

## リリース

タグを push すると GitHub Actions がバイナリとチェックサムを作り、Release を作成します。

```bash
git tag v2.0.0 && git push origin v2.0.0
```

手元で同じものを作る場合は `make dist` です（`dist/` に出力されます）。

## ライセンス

[LICENSE.md](LICENSE.md) を参照してください。

# Envoy Sidecar Proxy with xDS Sample Project

このプロジェクトは、Envoyをサイドカープロキシとして使用し、xDSサーバーで動的設定を行うサンプルアプリケーションです。

## 構成

- **xDS Server (Go)**: Envoyの動的設定を提供するコントロールプレーン
- **Spring Boot アプリケーション**: 外部APIを呼び出すREST APIを提供
- **Envoy Proxy**: IngressとEgressトラフィックを制御するサイドカープロキシ
- **Nginx**: 外部APIサーバーをシミュレート

## アーキテクチャ

```
xDS Server (Go) -> Envoy Config
Client -> Envoy (Ingress:8080) -> Spring Boot App -> Envoy (Egress:8081) -> Nginx External API
```

## 実行方法

1. プロジェクトをクローンまたはダウンロード
2. Docker Composeでサービスを起動:

```bash
docker-compose up --build
```

## エンドポイント

- **アプリケーション API**: `http://localhost:8080/api/data`
- **ヘルスチェック**: `http://localhost:8080/health`
- **Envoy Admin**: `http://localhost:9901`
- **xDS Server**: `localhost:18000` (gRPC)

## テスト

```bash
# アプリケーションのヘルスチェック
curl http://localhost:8080/health

# 外部API呼び出しテスト
curl http://localhost:8080/api/data
```

## ファイル構成

- `demo/`: Spring Bootアプリケーション
- `xds-server/`: Go製xDSサーバー（動的設定配信）
- `envoy/envoy-dynamic.yaml`: Envoy動的設定ファイル
- `envoy/envoy.yaml`: Envoy静的設定ファイル（旧版）
- `nginx/nginx.conf`: Nginx外部API設定
- `docker-compose.yml`: Docker Compose設定

## xDS機能

- **LDS (Listener Discovery Service)**: リスナー設定の動的配信
- **CDS (Cluster Discovery Service)**: クラスター設定の動的配信
- **RDS (Route Discovery Service)**: ルート設定の動的配信
- **リトライポリシー**: 5xx、接続エラー時の自動リトライ
- **サーキットブレーカー**: 過負荷時の保護機能
- **JSONログ出力**: 構造化されたアクセスログ
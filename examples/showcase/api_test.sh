#!/usr/bin/env bash
# =============================================================
# Showcase API — curl 调用示例
# 使用前确保服务已启动：
#   go run ./cmd/api/*.go
#   go run ./cmd/grpc/*.go   （gRPC 测试需要）
#   go run ./cmd/worker/*.go （worker 测试需要）
# =============================================================

cd "$(dirname "$0")"

API="http://localhost:8080"

GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

title() { echo -e "\n${BLUE}▶ $1${NC}"; }
ok()    { echo -e "${GREEN}✓ $1${NC}"; }

# json pretty-print，解析失败时原样输出
pp() { python3 -m json.tool 2>/dev/null || cat; }

# =============================================================
# 0. 读取数据库实际 ID
# =============================================================
title "读取数据库实际 ID"
export $(cat .env | grep -v '^#' | grep -v '^$' | xargs) 2>/dev/null || true

# Pre-build tools to avoid repeated go run compile overhead
DBINFO_BIN=$(mktemp /tmp/dbinfo.XXXXXX)
GENTOKEN_BIN=$(mktemp /tmp/gentoken.XXXXXX)
go build -o "$DBINFO_BIN"   ./tools/dbinfo/   2>/dev/null
go build -o "$GENTOKEN_BIN" ./tools/gentoken/ 2>/dev/null

DB_INFO=$("$DBINFO_BIN" 2>/dev/null)
if [ -z "$DB_INFO" ]; then
  echo "无法连接数据库，请检查 .env 里的 DATABASE_URL"
  rm -f "$DBINFO_BIN" "$GENTOKEN_BIN"
  exit 1
fi

# Look up by email so role changes during the test don't break ID resolution
ADMIN_ID=$(echo "$DB_INFO"  | grep 'email=admin@demo.local'  | grep -o 'id=[0-9]*' | cut -d= -f2)
SELLER_ID=$(echo "$DB_INFO" | grep 'email=seller@demo.local' | grep -o 'id=[0-9]*' | cut -d= -f2)
BUYER_ID=$(echo "$DB_INFO"  | grep 'email=buyer@demo.local'  | grep -o 'id=[0-9]*' | cut -d= -f2)
PRODUCT1=$(echo "$DB_INFO"  | grep 'stock='      | awk 'NR==1' | grep -o 'id=[0-9]*' | cut -d= -f2)
PRODUCT2=$(echo "$DB_INFO"  | grep 'stock='      | awk 'NR==2' | grep -o 'id=[0-9]*' | cut -d= -f2)
LOW_STOCK_ID=$(echo "$DB_INFO" | grep 'Rare Part' | grep -o 'id=[0-9]*' | cut -d= -f2)

ok "admin=$ADMIN_ID  seller=$SELLER_ID  buyer=$BUYER_ID"
ok "product1=$PRODUCT1  product2=$PRODUCT2  low_stock=$LOW_STOCK_ID"

# =============================================================
# 1. 生成 JWT Token
# =============================================================
title "生成 JWT Token"
ADMIN_TOKEN=$("$GENTOKEN_BIN" admin   "$ADMIN_ID")
SELLER_TOKEN=$("$GENTOKEN_BIN" seller "$SELLER_ID")
BUYER_TOKEN=$("$GENTOKEN_BIN" buyer   "$BUYER_ID")
ok "token 生成完成"

# =============================================================
# 2. 健康检查
# =============================================================
title "1. 健康检查"
curl -s "$API/health" | pp

# =============================================================
# 3. 商品接口
# =============================================================
title "2-1. 商品列表（buyer，第1页缓存）"
curl -s "$API/api/v1/products?page=1&page_size=5" \
  -H "Authorization: Bearer $BUYER_TOKEN" | pp

title "2-2. 创建商品（seller）"
NEW_PRODUCT=$(curl -s -X POST "$API/api/v1/products" \
  -H "Authorization: Bearer $SELLER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Test Widget","price":19.99,"stock":50,"category":"test"}')
echo "$NEW_PRODUCT" | pp
NEW_PRODUCT_ID=$(echo "$NEW_PRODUCT" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['id'])" 2>/dev/null || echo "")

title "2-3. 获取单个商品"
curl -s "$API/api/v1/products/$PRODUCT1" \
  -H "Authorization: Bearer $BUYER_TOKEN" | pp

title "2-4. 更新商品（seller）"
curl -s -X PUT "$API/api/v1/products/$PRODUCT1" \
  -H "Authorization: Bearer $SELLER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"price":24.99}' | pp

title "2-5. 删除商品（seller，删除刚创建的测试商品）"
if [ -n "$NEW_PRODUCT_ID" ]; then
  curl -s -X DELETE "$API/api/v1/products/$NEW_PRODUCT_ID" \
    -H "Authorization: Bearer $SELLER_TOKEN" -w "HTTP %{http_code}\n"
else
  echo "（跳过：商品创建失败）"
fi

# =============================================================
# 4. 订单接口
# =============================================================
title "3-1. 创建订单（buyer，原价）"
ORDER=$(curl -s -X POST "$API/api/v1/orders" \
  -H "Authorization: Bearer $BUYER_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"items\":[{\"product_id\":$PRODUCT1,\"qty\":1},{\"product_id\":$PRODUCT2,\"qty\":1}]}")
echo "$ORDER" | pp
ORDER_ID=$(echo "$ORDER" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['id'])" 2>/dev/null || echo "1")

title "3-2. 创建订单（Canary v2，享受 5% 折扣）"
curl -s -X POST "$API/api/v1/orders" \
  -H "Authorization: Bearer $BUYER_TOKEN" \
  -H "Content-Type: application/json" \
  -H "X-Canary: true" \
  -d "{\"items\":[{\"product_id\":$PRODUCT1,\"qty\":1}]}" | pp

title "3-3. 订单列表"
curl -s "$API/api/v1/orders?page=1&page_size=5" \
  -H "Authorization: Bearer $BUYER_TOKEN" | pp

title "3-4. 获取订单详情"
curl -s "$API/api/v1/orders/$ORDER_ID" \
  -H "Authorization: Bearer $BUYER_TOKEN" | pp

title "3-5. 库存不足时下单（预期 409）"
curl -s -X POST "$API/api/v1/orders" \
  -H "Authorization: Bearer $BUYER_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"items\":[{\"product_id\":$LOW_STOCK_ID,\"qty\":9999}]}" | pp

# =============================================================
# 5. 管理接口
# =============================================================
title "4-1. 获取用户信息（admin）"
curl -s "$API/api/v1/admin/users/$BUYER_ID" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | pp

title "4-2. 修改用户角色（admin）"
curl -s -X PUT "$API/api/v1/admin/users/$BUYER_ID/role" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"role":"seller"}' | pp

title "4-3. 非 admin 访问管理接口（预期 403）"
curl -s "$API/api/v1/admin/users/$ADMIN_ID" \
  -H "Authorization: Bearer $BUYER_TOKEN" | pp

# =============================================================
# 6. gRPC 接口
# =============================================================
title "5. gRPC 接口测试"
# Pre-build to avoid go run compile time consuming the RPC context deadline
GRPC_BIN=$(mktemp /tmp/grpcclient.XXXXXX)
if go build -o "$GRPC_BIN" ./tools/grpcclient/ 2>/dev/null; then
  # Pass explicit localhost address; GRPC_ADDR=:9091 from .env omits the host
  GRPC_ADDR=localhost:9091 "$GRPC_BIN" || echo "（gRPC 调用失败，请确认 cmd/grpc 已启动）"
  rm -f "$GRPC_BIN"
else
  echo "（跳过：grpcclient 编译失败）"
fi

echo -e "\n${GREEN}=== 全部完成 ===${NC}\n"

rm -f "$DBINFO_BIN" "$GENTOKEN_BIN"

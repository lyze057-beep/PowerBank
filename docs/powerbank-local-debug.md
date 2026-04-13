# PowerBank 本地联调示例

## 1. 准备工作

### 1.1 导入表结构和种子数据
按顺序执行：

```sql
source deploy/sql/001_users.sql;
source deploy/sql/002_payment_orders.sql;
source deploy/sql/003_wallet_balance.sql;
source deploy/sql/004_notification_records.sql;
source deploy/sql/005_support_chat.sql;
source deploy/sql/006_deposit.sql;
source deploy/sql/007_charger_inventory.sql;
source deploy/sql/008_rent_orders.sql;
source deploy/sql/009_local_seed_demo.sql;
```

### 1.2 启动服务
```powershell
go run .\cmd\server -conf .\configs
```

默认地址：`http://127.0.0.1:8000`

### 1.3 Postman 推荐变量
- `baseUrl` = `http://127.0.0.1:8000`
- `debugUid` = `u_debug_need_deposit`
- `stationA` = `STATION_DEMO_A`
- `stationB` = `STATION_DEMO_B`
- `returnSlot` = `SLOT_DEMO_B_01`
- `wxMockSign` = `MOCK_SIGN`
- `aliMockSign` = `MOCK_SIGN`

## 2. 测试账号
默认密码统一为：`Passw0rd!`

| UID | 手机号 | 场景 |
|---|---|---|
| `u_debug_need_deposit` | `13800000001` | 需要先缴押金 |
| `u_debug_exempt` | `13800000002` | 已免押，可直接借 |
| `u_debug_paid` | `13800000003` | 已缴押金，可直接借 |
| `u_debug_fee_case` | `13800000004` | 已有一笔借出中的订单，可直接测归还计费 |

## 3. 调试方式

### 3.1 推荐方式：直接用 `x-debug-uid`
除登录/注册接口外，其余需要鉴权的接口都可以加请求头：

```http
x-debug-uid: u_debug_paid
```

这样不用先走 JWT 登录，最适合 Postman 快速联调。

### 3.2 常规方式：先登录
```http
POST {{baseUrl}}/v1/auth/login
Content-Type: application/json

{
  "mobile": "13800000003",
  "password": "Passw0rd!"
}
```

登录成功后，把返回的 `accessToken` 放到请求头：

```http
Authorization: Bearer <access_token>
```

## 4. 第二周接口联调

### 4.1 查询押金状态
```http
GET {{baseUrl}}/v1/deposits/status
x-debug-uid: u_debug_need_deposit
```

### 4.2 发起免押申请
`provider`：`1=ALIPAY_CREDIT`，`2=WECHAT_CREDIT`

```http
POST {{baseUrl}}/v1/deposits/exemptions:apply
Content-Type: application/json
x-debug-uid: u_debug_need_deposit

{
  "provider": 1,
  "clientReqId": "dep-exempt-001"
}
```

说明：mock 信用网关按配置阈值审批，默认大多数 demo 用户可直接通过。

### 4.3 创建微信押金单
`channel`：`1=WECHAT`，`2=ALIPAY`
`payMode`：`1=NATIVE`，`2=JSAPI`

```http
POST {{baseUrl}}/v1/deposits/orders
Content-Type: application/json
x-debug-uid: u_debug_need_deposit

{
  "channel": 1,
  "payMode": 1,
  "clientReqId": "dep-order-001"
}
```

### 4.4 创建支付宝押金单
```http
POST {{baseUrl}}/v1/deposits/orders
Content-Type: application/json
x-debug-uid: u_debug_need_deposit

{
  "channel": 2,
  "payMode": 1,
  "clientReqId": "dep-order-ali-001"
}
```

### 4.5 查询押金记录
```http
GET {{baseUrl}}/v1/deposits/records?page=1&pageSize=20
x-debug-uid: u_debug_need_deposit
```

### 4.6 触发押金支付成功回调
把上一步创建押金单返回的 `outTradeNo` 填进去。

微信：
```http
POST {{baseUrl}}/v1/payments/wx/notify
Content-Type: application/json

{
  "body": "{\"event_id\":\"evt_dep_001\",\"event_type\":\"TRANSACTION.SUCCESS\",\"out_trade_no\":\"{{depositOutTradeNo}}\",\"trade_state\":\"SUCCESS\",\"transaction_id\":\"wx_txn_dep_001\"}",
  "timestamp": "1712121212",
  "nonce": "demo-nonce",
  "signature": "MOCK_SIGN",
  "serial": "demo-serial"
}
```

支付宝：
```http
POST {{baseUrl}}/v1/payments/alipay/notify
Content-Type: application/json

{
  "body": "{\"notify_id\":\"ali_dep_evt_001\",\"out_trade_no\":\"{{depositOutTradeNo}}\",\"trade_status\":\"TRADE_SUCCESS\",\"trade_no\":\"ali_trade_dep_001\"}",
  "signature": "MOCK_SIGN",
  "notifyId": "ali_dep_evt_001",
  "timestamp": "2026-04-03 10:00:00"
}
```

回调成功后，再调一次 `GET /v1/deposits/status`，应该能看到 `paid=true` 或 `status=PAID`。

## 5. 第三周接口联调

### 5.1 扫码借出
可直接用已免押或已缴押金用户：`u_debug_exempt` / `u_debug_paid`

```http
POST {{baseUrl}}/v1/chargers/scan:borrow
Content-Type: application/json
x-debug-uid: u_debug_paid

{
  "stationId": "STATION_DEMO_A",
  "clientReqId": "borrow-001"
}
```

### 5.2 借出结果回调
把上一步返回的 `rentOrderNo` 填进去。

```http
POST {{baseUrl}}/v1/internal/chargers/borrow:notify
Content-Type: application/json

{
  "rentOrderNo": "{{rentOrderNo}}",
  "success": true,
  "reason": ""
}
```

### 5.3 查询当前订单
```http
GET {{baseUrl}}/v1/orders/current
x-debug-uid: u_debug_paid
```

### 5.4 归还结果回调
快速归还场景：
```http
POST {{baseUrl}}/v1/internal/chargers/return:notify
Content-Type: application/json

{
  "rentOrderNo": "{{rentOrderNo}}",
  "stationId": "STATION_DEMO_B",
  "slotId": "SLOT_DEMO_B_01",
  "success": true,
  "reason": ""
}
```

### 5.5 查询订单列表
```http
GET {{baseUrl}}/v1/orders?page=1&pageSize=20
x-debug-uid: u_debug_paid
```

### 5.6 查询订单详情
```http
GET {{baseUrl}}/v1/orders/{{rentOrderNo}}
x-debug-uid: u_debug_paid
```

### 5.7 上报订单异常
```http
POST {{baseUrl}}/v1/orders/{{rentOrderNo}}:reportException
Content-Type: application/json
x-debug-uid: u_debug_paid

{
  "rentOrderNo": "{{rentOrderNo}}",
  "exceptionType": 1,
  "description": "归还时仓门未打开",
  "clientReqId": "order-exception-001"
}
```

## 6. 直接测试“归还后产生租金”场景
种子数据里已经准备好一笔借出中的订单：
- 用户：`u_debug_fee_case`
- 订单：`RO_FEE_ACTIVE_001`

直接调用归还回调：
```http
POST {{baseUrl}}/v1/internal/chargers/return:notify
Content-Type: application/json

{
  "rentOrderNo": "RO_FEE_ACTIVE_001",
  "stationId": "STATION_DEMO_B",
  "slotId": "SLOT_DEMO_B_02",
  "success": true,
  "reason": ""
}
```

然后查询：
```http
GET {{baseUrl}}/v1/orders/RO_FEE_ACTIVE_001
x-debug-uid: u_debug_fee_case
```

如果返回 `payStatus=UNPAID`、`status=PAY_PENDING`，说明归还计费链路正常。

## 7. 现有支付接口补租借订单支付
当订单进入 `PAY_PENDING` 后，继续复用已有支付接口。
`bizType`：`2=RENT_ORDER`

```http
POST {{baseUrl}}/v1/payments/wx/orders
Content-Type: application/json
x-debug-uid: u_debug_fee_case

{
  "payMode": 1,
  "bizType": 2,
  "bizOrderNo": "RO_FEE_ACTIVE_001",
  "amount": 4500,
  "clientReqId": "rent-pay-001"
}
```

再回调支付成功：
```http
POST {{baseUrl}}/v1/payments/wx/notify
Content-Type: application/json

{
  "body": "{\"event_id\":\"evt_rent_001\",\"event_type\":\"TRANSACTION.SUCCESS\",\"out_trade_no\":\"{{rentPayOutTradeNo}}\",\"trade_state\":\"SUCCESS\",\"transaction_id\":\"wx_txn_rent_001\"}",
  "timestamp": "1712121212",
  "nonce": "demo-nonce",
  "signature": "MOCK_SIGN",
  "serial": "demo-serial"
}
```

## 8. 柜机与槽位测试数据
- 借出柜机：`STATION_DEMO_A`
- 归还柜机：`STATION_DEMO_B`
- 可归还空槽：`SLOT_DEMO_B_01`
- 计费测试空槽：`SLOT_DEMO_B_02`

## 9. Postman Collection
如果你想直接导入 Postman，可使用旁边这份 collection：
- `docs/powerbank-local-debug.postman_collection.json`

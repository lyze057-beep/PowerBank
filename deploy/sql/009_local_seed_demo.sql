-- 本文件仅用于本地联调示例数据。
-- 请先执行 001 ~ 008 建表 SQL，再执行本文件。

SET NAMES utf8mb4;

-- 1. 清理固定 demo 数据，支持重复执行
DELETE FROM rent_order_events WHERE rent_order_no IN ('RO_HISTORY_001', 'RO_FEE_ACTIVE_001');
DELETE FROM rent_orders WHERE rent_order_no IN ('RO_HISTORY_001', 'RO_FEE_ACTIVE_001');
DELETE FROM station_slots WHERE slot_id IN ('SLOT_DEMO_A_01', 'SLOT_DEMO_A_02', 'SLOT_DEMO_B_01', 'SLOT_DEMO_B_02');
DELETE FROM powerbanks WHERE powerbank_id IN ('PB_DEMO_001', 'PB_DEMO_002', 'PB_DEMO_003', 'PB_BORROWED_001');
DELETE FROM stations WHERE station_id IN ('STATION_DEMO_A', 'STATION_DEMO_B');
DELETE FROM pricing_rules WHERE rule_id IN ('PR_DEMO_STANDARD', 'PR_DEMO_FAST');
DELETE FROM deposit_exemption_records WHERE exemption_id IN ('EX_DEMO_001');
DELETE FROM deposit_orders WHERE deposit_order_no IN ('DP_DEMO_PAID_001', 'DP_DEMO_FEE_001');
DELETE FROM payment_orders WHERE out_trade_no IN ('DA_DEMO_PAID_001', 'DA_DEMO_FEE_001', 'RW_HISTORY_001');
DELETE FROM payment_events WHERE out_trade_no IN ('DA_DEMO_PAID_001', 'DA_DEMO_FEE_001', 'RW_HISTORY_001');
DELETE FROM deposit_profiles WHERE uid IN ('u_debug_need_deposit', 'u_debug_exempt', 'u_debug_paid', 'u_debug_fee_case');
DELETE FROM user_wallets WHERE uid IN ('u_debug_need_deposit', 'u_debug_exempt', 'u_debug_paid', 'u_debug_fee_case');
DELETE FROM users WHERE uid IN ('u_debug_need_deposit', 'u_debug_exempt', 'u_debug_paid', 'u_debug_fee_case');

-- 2. 基础用户
INSERT INTO users(uid, mobile, password_hash, nickname, avatar, status, last_login_at)
VALUES
('u_debug_need_deposit', '13800000001', '$2a$10$IAdA/JBSkH/aBKS9EXi1Vu95DVlxefZzinuPi0xaIACULcgG6VIm2', '待押金用户', '', 1, NOW(3)),
('u_debug_exempt',       '13800000002', '$2a$10$IAdA/JBSkH/aBKS9EXi1Vu95DVlxefZzinuPi0xaIACULcgG6VIm2', '免押用户',   '', 1, NOW(3)),
('u_debug_paid',         '13800000003', '$2a$10$IAdA/JBSkH/aBKS9EXi1Vu95DVlxefZzinuPi0xaIACULcgG6VIm2', '已押金用户', '', 1, NOW(3)),
('u_debug_fee_case',     '13800000004', '$2a$10$IAdA/JBSkH/aBKS9EXi1Vu95DVlxefZzinuPi0xaIACULcgG6VIm2', '计费场景用户', '', 1, NOW(3));

-- 3. 钱包
INSERT INTO user_wallets(uid, balance)
VALUES
('u_debug_need_deposit', 0),
('u_debug_exempt', 5000),
('u_debug_paid', 8000),
('u_debug_fee_case', 1200);

-- 4. 押金档案
INSERT INTO deposit_profiles(uid, status, deposit_amount, paid, exempt, active_deposit_order_no, exempt_provider, exempt_expire_at)
VALUES
('u_debug_need_deposit', 1, 9900, 0, 0, '', '', NULL),
('u_debug_exempt',       3, 9900, 0, 1, '', 'ALIPAY_CREDIT', DATE_ADD(NOW(3), INTERVAL 30 DAY)),
('u_debug_paid',         2, 9900, 1, 0, 'DP_DEMO_PAID_001', '', NULL),
('u_debug_fee_case',     2, 9900, 1, 0, 'DP_DEMO_FEE_001', '', NULL);

INSERT INTO deposit_exemption_records(exemption_id, uid, client_req_id, provider, credit_score, status, reason, expire_at)
VALUES
('EX_DEMO_001', 'u_debug_exempt', 'seed_exempt_001', 'ALIPAY_CREDIT', 705, 2, 'seed approved', DATE_ADD(NOW(3), INTERVAL 30 DAY));

-- 5. 已完成押金订单
INSERT INTO payment_orders(uid, out_trade_no, client_req_id, channel, pay_mode, biz_type, biz_order_no, amount, status, prepay_id, code_url, jsapi_params, transaction_id, failed_reason, paid_at)
VALUES
('u_debug_paid',     'DA_DEMO_PAID_001', 'seed_dep_paid_001', 'ALIPAY', 1, 3, 'DP_DEMO_PAID_001', 9900, 3, 'ali_prepay_paid_001', 'https://mock/pay/paid', JSON_OBJECT(), 'ali_txn_paid_001', '', NOW(3)),
('u_debug_fee_case', 'DA_DEMO_FEE_001',  'seed_dep_fee_001',  'ALIPAY', 1, 3, 'DP_DEMO_FEE_001',  9900, 3, 'ali_prepay_fee_001',  'https://mock/pay/fee',  JSON_OBJECT(), 'ali_txn_fee_001', '', NOW(3)),
('u_debug_paid',     'RW_HISTORY_001',   'seed_rent_paid_001', 'WECHAT', 1, 2, 'RO_HISTORY_001',   400,  3, 'wx_prepay_history_001', 'weixin://mock/history', JSON_OBJECT(), 'wx_txn_history_001', '', DATE_SUB(NOW(3), INTERVAL 1 DAY));

INSERT INTO deposit_orders(deposit_order_no, uid, out_trade_no, client_req_id, channel, pay_mode, amount, status)
VALUES
('DP_DEMO_PAID_001', 'u_debug_paid', 'DA_DEMO_PAID_001', 'seed_dep_paid_001', 'ALIPAY', 1, 9900, 3),
('DP_DEMO_FEE_001',  'u_debug_fee_case', 'DA_DEMO_FEE_001', 'seed_dep_fee_001', 'ALIPAY', 1, 9900, 3);

-- 6. 计费规则与柜机
INSERT INTO pricing_rules(rule_id, name, free_minutes, unit_minutes, unit_price, daily_cap)
VALUES
('PR_DEMO_STANDARD', '演示标准计费', 10, 30, 200, 2999),
('PR_DEMO_FAST',     '演示快速计费', 0, 1, 100, 9999);

INSERT INTO stations(station_id, name, status, pricing_rule_id)
VALUES
('STATION_DEMO_A', '演示柜机A', 1, 'PR_DEMO_STANDARD'),
('STATION_DEMO_B', '演示柜机B', 1, 'PR_DEMO_FAST');

INSERT INTO powerbanks(powerbank_id, status, current_station_id, current_slot_id)
VALUES
('PB_DEMO_001', 1, 'STATION_DEMO_A', 'SLOT_DEMO_A_01'),
('PB_DEMO_002', 1, 'STATION_DEMO_A', 'SLOT_DEMO_A_02'),
('PB_DEMO_003', 1, 'STATION_DEMO_B', 'SLOT_DEMO_B_02'),
('PB_BORROWED_001', 2, '', '');

INSERT INTO station_slots(slot_id, station_id, powerbank_id, status)
VALUES
('SLOT_DEMO_A_01', 'STATION_DEMO_A', 'PB_DEMO_001', 1),
('SLOT_DEMO_A_02', 'STATION_DEMO_A', 'PB_DEMO_002', 1),
('SLOT_DEMO_B_01', 'STATION_DEMO_B', '', 1),
('SLOT_DEMO_B_02', 'STATION_DEMO_B', 'PB_DEMO_003', 1);

-- 7. 历史完成订单，方便查列表/详情
INSERT INTO rent_orders(
  uid, rent_order_no, client_req_id, station_id, return_station_id, powerbank_id, borrow_slot_id, return_slot_id,
  pricing_rule_id, status, pay_status, deposit_amount, rent_fee, payment_out_trade_no, borrowed_at, returned_at, exception_reported, exception_desc
)
VALUES
(
  'u_debug_paid', 'RO_HISTORY_001', 'seed_history_order_001',
  'STATION_DEMO_A', 'STATION_DEMO_B', 'PB_DEMO_999', 'SLOT_DEMO_A_99', 'SLOT_DEMO_B_99',
  'PR_DEMO_STANDARD', 5, 3, 9900, 400, 'RW_HISTORY_001',
  DATE_SUB(NOW(3), INTERVAL 1 DAY), DATE_SUB(NOW(3), INTERVAL 23 HOUR), 0, ''
);

INSERT INTO rent_order_events(rent_order_no, event_type, event_key, payload)
VALUES
('RO_HISTORY_001', 'BORROW_CALLBACK', 'seed:history:borrow', JSON_OBJECT('success', true)),
('RO_HISTORY_001', 'RETURN_CALLBACK', 'seed:history:return', JSON_OBJECT('success', true));

-- 8. 一笔借出中的订单，方便直接测归还和计费
INSERT INTO rent_orders(
  uid, rent_order_no, client_req_id, station_id, return_station_id, powerbank_id, borrow_slot_id, return_slot_id,
  pricing_rule_id, status, pay_status, deposit_amount, rent_fee, payment_out_trade_no, borrowed_at, returned_at, exception_reported, exception_desc
)
VALUES
(
  'u_debug_fee_case', 'RO_FEE_ACTIVE_001', 'seed_active_fee_001',
  'STATION_DEMO_B', '', 'PB_BORROWED_001', 'SLOT_DEMO_B_X', '',
  'PR_DEMO_FAST', 2, 1, 9900, 0, '',
  DATE_SUB(NOW(3), INTERVAL 45 MINUTE), NULL, 0, ''
);

INSERT INTO rent_order_events(rent_order_no, event_type, event_key, payload)
VALUES
('RO_FEE_ACTIVE_001', 'BORROW_CALLBACK', 'seed:fee:borrow', JSON_OBJECT('success', true));

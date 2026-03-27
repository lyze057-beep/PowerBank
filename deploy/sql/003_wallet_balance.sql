CREATE TABLE IF NOT EXISTS `user_wallets` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `uid` VARCHAR(64) NOT NULL COMMENT '业务用户ID',
  `balance` BIGINT NOT NULL DEFAULT 0 COMMENT '余额(分)',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_wallets_uid` (`uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `wallet_recharge_records` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `uid` VARCHAR(64) NOT NULL COMMENT '业务用户ID',
  `out_trade_no` VARCHAR(64) NOT NULL COMMENT '支付单号',
  `channel` VARCHAR(16) NOT NULL COMMENT 'WECHAT/ALIPAY',
  `amount` BIGINT NOT NULL COMMENT '充值金额(分)',
  `status` TINYINT NOT NULL DEFAULT 1 COMMENT '1成功',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_wallet_recharge_records_out_trade_no` (`out_trade_no`),
  KEY `idx_wallet_recharge_records_uid` (`uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

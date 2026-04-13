CREATE TABLE IF NOT EXISTS `deposit_profiles` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `uid` VARCHAR(64) NOT NULL,
  `status` TINYINT NOT NULL DEFAULT 1 COMMENT '1=REQUIRED,2=PAID,3=EXEMPT',
  `deposit_amount` BIGINT NOT NULL DEFAULT 9900,
  `paid` TINYINT NOT NULL DEFAULT 0,
  `exempt` TINYINT NOT NULL DEFAULT 0,
  `active_deposit_order_no` VARCHAR(64) NOT NULL DEFAULT '',
  `exempt_provider` VARCHAR(32) NOT NULL DEFAULT '',
  `exempt_expire_at` DATETIME(3) NULL DEFAULT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_deposit_profiles_uid` (`uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `deposit_orders` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `uid` VARCHAR(64) NOT NULL,
  `deposit_order_no` VARCHAR(64) NOT NULL,
  `out_trade_no` VARCHAR(64) NOT NULL,
  `client_req_id` VARCHAR(64) NOT NULL,
  `channel` VARCHAR(16) NOT NULL,
  `pay_mode` TINYINT NOT NULL DEFAULT 1,
  `amount` BIGINT NOT NULL,
  `status` TINYINT NOT NULL DEFAULT 1 COMMENT '1=INIT,2=PAYING,3=SUCCESS,4=FAILED,5=REFUNDED',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_deposit_orders_order_no` (`deposit_order_no`),
  UNIQUE KEY `uk_deposit_orders_out_trade_no` (`out_trade_no`),
  UNIQUE KEY `uk_deposit_orders_uid_req` (`uid`, `client_req_id`),
  KEY `idx_deposit_orders_uid_created` (`uid`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `deposit_exemption_records` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `uid` VARCHAR(64) NOT NULL,
  `exemption_id` VARCHAR(64) NOT NULL,
  `client_req_id` VARCHAR(64) NOT NULL,
  `provider` VARCHAR(32) NOT NULL,
  `credit_score` INT NOT NULL DEFAULT 0,
  `status` TINYINT NOT NULL DEFAULT 1 COMMENT '1=PENDING,2=APPROVED,3=REJECTED',
  `reason` VARCHAR(255) NOT NULL DEFAULT '',
  `expire_at` DATETIME(3) NULL DEFAULT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_deposit_exemption_id` (`exemption_id`),
  UNIQUE KEY `uk_deposit_exemption_uid_req` (`uid`, `client_req_id`),
  KEY `idx_deposit_exemption_uid_created` (`uid`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

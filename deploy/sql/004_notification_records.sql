CREATE TABLE IF NOT EXISTS `notification_records` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `uid` VARCHAR(64) NOT NULL COMMENT 'business user id',
  `title` VARCHAR(128) NOT NULL DEFAULT '',
  `content` VARCHAR(1000) NOT NULL DEFAULT '',
  `biz_type` VARCHAR(64) NOT NULL DEFAULT '',
  `biz_id` VARCHAR(64) NOT NULL DEFAULT '',
  `client_req_id` VARCHAR(64) NOT NULL COMMENT 'client idempotent request id',
  `topic` VARCHAR(255) NOT NULL DEFAULT '',
  `status` TINYINT NOT NULL DEFAULT 1 COMMENT '1=INIT,2=SENT,3=FAILED',
  `failed_reason` VARCHAR(255) NOT NULL DEFAULT '',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_notification_records_uid_req` (`uid`, `client_req_id`),
  KEY `idx_notification_records_uid_id` (`uid`, `id`),
  KEY `idx_notification_records_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

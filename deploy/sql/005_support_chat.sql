CREATE TABLE IF NOT EXISTS `support_sessions` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `uid` VARCHAR(64) NOT NULL COMMENT 'business user id',
  `session_id` VARCHAR(64) NOT NULL COMMENT 'chat session id',
  `status` TINYINT NOT NULL DEFAULT 1 COMMENT '1=active,2=closed',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_support_sessions_session_id` (`session_id`),
  KEY `idx_support_sessions_uid` (`uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `support_turns` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `uid` VARCHAR(64) NOT NULL COMMENT 'business user id',
  `session_id` VARCHAR(64) NOT NULL COMMENT 'chat session id',
  `client_req_id` VARCHAR(64) NOT NULL COMMENT 'client idempotent request id',
  `user_message` TEXT NOT NULL,
  `assistant_reply` TEXT NOT NULL,
  `intent` VARCHAR(32) NOT NULL DEFAULT 'UNKNOWN',
  `used_fallback` TINYINT NOT NULL DEFAULT 0 COMMENT '0=no,1=yes',
  `model_name` VARCHAR(128) NOT NULL DEFAULT '',
  `prompt_tokens` INT NOT NULL DEFAULT 0,
  `reply_tokens` INT NOT NULL DEFAULT 0,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_support_turns_uid_req` (`uid`, `client_req_id`),
  KEY `idx_support_turns_session_id_id` (`session_id`, `id`),
  KEY `idx_support_turns_uid_session` (`uid`, `session_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

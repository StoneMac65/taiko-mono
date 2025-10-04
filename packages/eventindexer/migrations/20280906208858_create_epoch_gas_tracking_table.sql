-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS epoch_gas_tracking (
    id int NOT NULL PRIMARY KEY AUTO_INCREMENT,
    epoch_id BIGINT NOT NULL,
    chain_id int NOT NULL,
    total_gas_used BIGINT NOT NULL DEFAULT 0,
    block_count int NOT NULL DEFAULT 0,
    avg_gas_per_block DECIMAL(20, 2) NOT NULL DEFAULT 0.00,
    min_gas BIGINT NOT NULL DEFAULT 0,
    max_gas BIGINT NOT NULL DEFAULT 0,
    first_block_id BIGINT NOT NULL,
    last_block_id BIGINT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY `epoch_chain_unique` (`epoch_id`, `chain_id`),
    INDEX `epoch_id_index` (`epoch_id`),
    INDEX `chain_id_index` (`chain_id`),
    INDEX `epoch_chain_index` (`epoch_id`, `chain_id`)
);

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP TABLE epoch_gas_tracking;
-- +goose StatementEnd

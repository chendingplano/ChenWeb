-- +goose Up
ALTER TABLE cust_request_logs RENAME COLUMN "desc" TO description;

-- +goose Down
ALTER TABLE cust_request_logs RENAME COLUMN description TO "desc";

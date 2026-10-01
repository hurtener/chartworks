-- Synthetic adversarial warehouse. Numeric IDs repeat across divisions by design.
CREATE TABLE analytics.adv_customers (
 division_id text NOT NULL, customer_id integer NOT NULL, segment text,
 PRIMARY KEY(division_id,customer_id)
);
CREATE TABLE analytics.adv_orders (
 division_id text NOT NULL, order_id integer NOT NULL, customer_id integer NOT NULL,
 ordered_at timestamptz NOT NULL, misleading_net_total_usd numeric(18,2),
 status_code text, private_note text NOT NULL,
 PRIMARY KEY(division_id,order_id),
 FOREIGN KEY(division_id,customer_id) REFERENCES analytics.adv_customers(division_id,customer_id)
);
COMMENT ON COLUMN analytics.adv_orders.misleading_net_total_usd IS
 'Legacy misleading name: this is booked gross before refunds, not net';
CREATE TABLE analytics.adv_refunds (
 division_id text NOT NULL, refund_id integer NOT NULL, order_id integer NOT NULL,
 refunded_at timestamptz NOT NULL, amount_usd numeric(18,2), status_code text,
 PRIMARY KEY(division_id,refund_id),
 FOREIGN KEY(division_id,order_id) REFERENCES analytics.adv_orders(division_id,order_id)
);
CREATE TABLE analytics.adv_order_lines (
 division_id text NOT NULL, line_id integer NOT NULL, order_id integer NOT NULL,
 category text NOT NULL, line_total_usd numeric(18,2) NOT NULL,
 PRIMARY KEY(division_id,line_id),
 FOREIGN KEY(division_id,order_id) REFERENCES analytics.adv_orders(division_id,order_id)
);

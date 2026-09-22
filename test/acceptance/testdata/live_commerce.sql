-- Synthetic commerce warehouse for the opt-in paid gateway gate.
-- All money is USD. orders.total_usd is the order grain; items and refunds
-- are one-to-many children and must be aggregated before joining to orders.
CREATE TABLE analytics.customers (
    customer_id integer PRIMARY KEY,
    segment text NOT NULL CHECK (segment IN ('consumer','business')),
    region text NOT NULL CHECK (region IN ('north','south','west'))
);
CREATE TABLE analytics.orders (
    order_id integer PRIMARY KEY,
    customer_id integer NOT NULL REFERENCES analytics.customers(customer_id),
    ordered_at date NOT NULL,
    total_usd numeric(12,2) NOT NULL CHECK (total_usd >= 0),
    status text NOT NULL CHECK (status IN ('paid','cancelled'))
);
CREATE TABLE analytics.order_items (
    item_id integer PRIMARY KEY,
    order_id integer NOT NULL REFERENCES analytics.orders(order_id),
    category text NOT NULL CHECK (category IN ('apparel','home','electronics')),
    quantity integer NOT NULL CHECK (quantity > 0),
    amount_usd numeric(12,2) NOT NULL CHECK (amount_usd >= 0)
);
CREATE TABLE analytics.refunds (
    refund_id integer PRIMARY KEY,
    order_id integer NOT NULL REFERENCES analytics.orders(order_id),
    refunded_at date NOT NULL,
    amount_usd numeric(12,2) NOT NULL CHECK (amount_usd >= 0)
);
INSERT INTO analytics.customers VALUES
    (1,'consumer','north'), (2,'business','south'),
    (3,'consumer','west'), (4,'business','north');
INSERT INTO analytics.orders VALUES
    (101,1,'2026-01-10',120.00,'paid'),
    (102,2,'2026-01-22',200.00,'paid'),
    (103,3,'2026-02-05',90.00,'paid'),
    (104,4,'2026-02-18',150.00,'paid'),
    (105,1,'2026-03-03',80.00,'paid'),
    (106,2,'2026-03-12',60.00,'cancelled');
INSERT INTO analytics.order_items VALUES
    (1001,101,'apparel',2,70.00), (1002,101,'home',1,50.00),
    (1003,102,'electronics',1,200.00),
    (1004,103,'apparel',1,90.00),
    (1005,104,'home',2,100.00), (1006,104,'apparel',1,50.00),
    (1007,105,'electronics',1,80.00), (1008,106,'home',1,60.00);
INSERT INTO analytics.refunds VALUES
    (201,101,'2026-01-25',20.00),
    (202,102,'2026-02-02',50.00),
    (203,102,'2026-02-04',25.00),
    (204,104,'2026-03-01',30.00);
-- Paid-order gross = 640.00; refunds against paid orders = 125.00;
-- net = 515.00. Joining items and refunds directly duplicates child rows.

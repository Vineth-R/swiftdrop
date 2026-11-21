CREATE TABLE IF NOT EXISTS notification.notification_logs (
  id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id      UUID         NOT NULL,
  parcel_id     UUID         NOT NULL,
  channel       VARCHAR(10)  NOT NULL CHECK (channel IN ('EMAIL','SMS')),
  recipient     VARCHAR(255) NOT NULL,                   -- mask in logs
  subject       VARCHAR(200),
  status        VARCHAR(10)  NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','SENT','FAILED')),
  error_message VARCHAR(255),
  created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
  sent_at       TIMESTAMPTZ,
  CONSTRAINT notification_dedupe_uq UNIQUE (event_id, channel, recipient)
);


CREATE TABLE IF NOT EXISTS notification.processed_events (
  consumer     VARCHAR(40) NOT NULL,
  event_id     UUID        NOT NULL,
  processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (consumer, event_id)
);

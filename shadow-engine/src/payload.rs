use serde::Deserialize;

#[derive(Deserialize, Debug)]
pub struct InboundTradePayload {
    pub agent_id: String,
    pub timestamp: i64,
    pub asset_class: String,
    pub ticker: String,
    pub order_type: String,
    pub quantity: f64,
    pub price: f64,
    pub context_window_reasoning: String,
    pub crypto_checksum: String,
}

impl InboundTradePayload {
    pub fn get_stack_ticker(&self) -> [u8; 8] {
        let bytes = self.ticker.as_bytes();
        let mut fixed_ticker = [0u8; 8];

        let len = bytes.len().min(8);
        fixed_ticker[..len].copy_from_slice(&bytes[..len]);

        fixed_ticker
    }
}

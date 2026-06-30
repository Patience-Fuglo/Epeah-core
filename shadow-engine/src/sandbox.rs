use std::collections::HashMap;
use std::sync::Arc;

#[derive(Clone, Copy, Debug)]
pub struct PriceLevel {
    pub price: u64,
    pub quantity: u64,
}

#[derive(Clone, Debug)]
pub struct MarketState {
    pub ticker: [u8; 8],
    pub bids: [PriceLevel; 10],
    pub asks: [PriceLevel; 10],
}

pub struct ShadowSandbox {
    pub sandbox_id: u32,
    pub base_state: Arc<HashMap<[u8; 8], MarketState>>,
    pub local_mutations: HashMap<[u8; 8], MarketState>,
}

impl ShadowSandbox {
    pub fn get_market_state(&self, ticker: &[u8; 8]) -> Option<&MarketState> {
        if let Some(mutated) = self.local_mutations.get(ticker) {
            Some(mutated)
        } else {
            self.base_state.get(ticker)
        }
    }
}

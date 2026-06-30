use futures::StreamExt;
use std::sync::{Arc, RwLock};
use std::collections::HashMap;
use serde::Deserialize;

use crate::sandbox::{MarketState, PriceLevel};

#[derive(Deserialize, Debug)]
pub struct MarketUpdatePacket {
    pub symbol: String,
    pub top_bid: u64,
    pub top_ask: u64,
    pub volume: u64,
}

pub struct LiveOrderBookHook {
    redis_url: String,
    pub state_registry: Arc<RwLock<HashMap<[u8; 8], MarketState>>>,
}

impl LiveOrderBookHook {
    pub fn new(url: &str, shared_state: Arc<RwLock<HashMap<[u8; 8], MarketState>>>) -> Self {
        Self {
            redis_url: url.to_string(),
            state_registry: shared_state,
        }
    }

    pub async fn start_streaming(&self) -> Result<(), Box<dyn std::error::Error>> {
        let client = redis::Client::open(self.redis_url.as_str())?;
        let mut pubsub = client.get_async_pubsub().await?;

        pubsub.subscribe("market_updates").await?;
        let mut stream = pubsub.on_message();

        let registry = self.state_registry.clone();

        println!("[LIVE HOOK] Linked to Redis Cluster. Ingesting live market ticks...");

        tokio::spawn(async move {
            while let Some(msg) = stream.next().await {
                let payload: String = match msg.get_payload() {
                    Ok(p) => p,
                    Err(_) => continue,
                };

                if let Ok(packet) = serde_json::from_str::<MarketUpdatePacket>(&payload) {
                    let mut ticker_bytes = [0u8; 8];
                    let src_bytes = packet.symbol.as_bytes();
                    let len = src_bytes.len().min(8);
                    ticker_bytes[..len].copy_from_slice(&src_bytes[..len]);

                    if let Ok(mut reg) = registry.write() {
                        let state = reg.entry(ticker_bytes).or_insert(MarketState {
                            ticker: ticker_bytes,
                            bids: [PriceLevel { price: 0, quantity: 0 }; 10],
                            asks: [PriceLevel { price: 0, quantity: 0 }; 10],
                        });

                        state.bids[0].price = packet.top_bid;
                        state.bids[0].quantity = packet.volume;
                        state.asks[0].price = packet.top_ask;
                        state.asks[0].quantity = packet.volume;
                    }
                }
            }
        });

        Ok(())
    }
}

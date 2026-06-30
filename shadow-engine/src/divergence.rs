use crate::payload::InboundTradePayload;
use crate::sandbox::ShadowSandbox;

pub struct DivergenceEngine {
    max_slippage_delta: f64,
    max_drawdown_percent: f64,
}

impl DivergenceEngine {
    pub fn new(max_slippage: f64, max_drawdown: f64) -> Self {
        Self {
            max_slippage_delta: max_slippage,
            max_drawdown_percent: max_drawdown,
        }
    }

    pub fn evaluate_deviation(
        &self,
        incoming: &InboundTradePayload,
        sandbox: &ShadowSandbox,
    ) -> Result<bool, String> {
        let ticker_bytes = incoming.get_stack_ticker();

        let market_state = match sandbox.get_market_state(&ticker_bytes) {
            Some(state) => state,
            None => return Err(format!("Unmapped ticker execution detected: {:?}", ticker_bytes)),
        };

        let live_price = incoming.price;
        let reference_price = market_state.asks[0].price as f64 / 10000.0;

        if reference_price == 0.0 {
            return Err("Invalid reference market price (zero value)".to_string());
        }

        let slippage = (live_price - reference_price).abs() / reference_price;

        if slippage > self.max_slippage_delta {
            println!(
                "[CRITICAL ALERT] Divergence detected on ticker {:?}. Slippage: {:.4}. Threshold: {:.4}",
                std::str::from_utf8(&ticker_bytes).unwrap_or("ERR"),
                slippage,
                self.max_slippage_delta
            );
            return Ok(true);
        }

        Ok(false)
    }
}

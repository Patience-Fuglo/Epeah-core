mod config;
mod divergence;
#[cfg(feature = "enclave")]
mod enclave_vsock;
mod guardrail;
mod ipc;
mod ledger;
mod live_feed;
mod payload;
mod sandbox;
mod signal;

use std::sync::{Arc, RwLock};
use std::collections::HashMap;

use config::ArbiterConfig;
#[cfg(feature = "enclave")]
use enclave_vsock::EnclaveVsockListener;
use ipc::RustIpcListener;
use live_feed::LiveOrderBookHook;

const CONFIG_PATH: &str = "/app/config.yaml";
const DEFAULT_REDIS_URL: &str = "redis://127.0.0.1:6379";
#[cfg(feature = "enclave")]
const ENCLAVE_VSOCK_PORT: u32 = 5005;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let environment = std::env::var("ARBITER_ENV").unwrap_or_else(|_| "DEV".to_string());

    println!("=========================================================");
    println!("ARBITER SHADOW ENGINE: INITIALIZING [{}]", environment);
    println!("=========================================================");

    let config = ArbiterConfig::load(CONFIG_PATH)?;

    println!(
        "Config loaded: mode={}, slippage_threshold={}, max_drawdown={}%",
        config.engine.mode,
        config.slippage_threshold(),
        config.max_drawdown_percent()
    );

    let slippage = config.slippage_threshold();
    let drawdown = config.max_drawdown_percent();

    let live_state = Arc::new(RwLock::new(HashMap::new()));

    let redis_url = std::env::var("ARBITER_REDIS_URL").unwrap_or_else(|_| DEFAULT_REDIS_URL.to_string());
    let feed = LiveOrderBookHook::new(&redis_url, live_state.clone());
    match feed.start_streaming().await {
        Ok(_) => println!("[LIVE HOOK] Redis market feed connected at {}", redis_url),
        Err(e) => println!("[LIVE HOOK] Redis not available ({}). Running with empty order book.", e),
    }

    #[cfg(feature = "enclave")]
    {
        if environment == "PROD" {
            println!("[HARDWARE MODE] Initializing AWS Nitro Hypervisor vsock layer on port {}...", ENCLAVE_VSOCK_PORT);
            let listener = EnclaveVsockListener::new(ENCLAVE_VSOCK_PORT, slippage, drawdown);
            return listener.run().await;
        }
    }

    let socket_path = config.engine.socket_path.clone();
    println!("[LOCAL DEV MODE] Initializing low-latency local Unix socket transport at {}", socket_path);
    let listener = RustIpcListener::new(&socket_path, slippage, drawdown, live_state);
    listener.run().await
}

use tokio::net::UnixListener;
use tokio_util::codec::{FramedRead, LinesCodec};
use futures::StreamExt;
use std::os::unix::fs::PermissionsExt;
use std::sync::{Arc, RwLock};
use std::collections::HashMap;

use crate::divergence::DivergenceEngine;
use crate::guardrail::ContextGuardrail;
use crate::ledger::AuditLedger;
use crate::payload::InboundTradePayload;
use crate::sandbox::{MarketState, ShadowSandbox};
use crate::signal::KillSignalPayload;

pub struct RustIpcListener {
    socket_path: String,
    slippage_threshold: f64,
    max_drawdown: f64,
    live_state: Arc<RwLock<HashMap<[u8; 8], MarketState>>>,
}

impl RustIpcListener {
    pub fn new(
        path: &str,
        slippage_threshold: f64,
        max_drawdown: f64,
        live_state: Arc<RwLock<HashMap<[u8; 8], MarketState>>>,
    ) -> Self {
        Self {
            socket_path: path.to_string(),
            slippage_threshold,
            max_drawdown,
            live_state,
        }
    }

    pub async fn run(&self) -> Result<(), Box<dyn std::error::Error>> {
        let _ = tokio::fs::remove_file(&self.socket_path).await;

        let listener = UnixListener::bind(&self.socket_path)?;

        let mut perms = tokio::fs::metadata(&self.socket_path).await?.permissions();
        perms.set_mode(0o600);
        tokio::fs::set_permissions(&self.socket_path, perms).await?;

        println!("Arbiter Rust Layer: Listening on IPC socket {}", self.socket_path);

        let slippage = self.slippage_threshold;
        let drawdown = self.max_drawdown;
        let shared_state = self.live_state.clone();

        while let Ok((stream, _)) = listener.accept().await {
            let state_ref = shared_state.clone();

            tokio::spawn(async move {
                let (read_half, mut write_half) = stream.into_split();
                let mut framed = FramedRead::new(read_half, LinesCodec::new());
                let mut ledger = AuditLedger::new("/var/log/arbiter/compliance_ledger.json");

                while let Some(Ok(line)) = framed.next().await {
                    if let Ok(payload) = serde_json::from_str::<InboundTradePayload>(&line) {
                        if ContextGuardrail::is_hallucinating_or_compromised(&payload.context_window_reasoning) {
                            let _ = ledger.commit_entry(&payload.agent_id, &payload.ticker, "REJECTED_SEMANTIC", &line);
                            let _ = KillSignalPayload::dispatch(
                                &mut write_half,
                                payload.agent_id,
                                payload.ticker,
                                "Semantic Guardrail Violation: Hallucination or Injection Loop Detected".to_string(),
                            ).await;
                            continue;
                        }

                        let engine = DivergenceEngine::new(slippage, drawdown);

                        let base_state = {
                            let reg = state_ref.read().unwrap_or_else(|e| e.into_inner());
                            Arc::new(reg.clone())
                        };

                        let sandbox = ShadowSandbox {
                            sandbox_id: 1,
                            base_state,
                            local_mutations: HashMap::new(),
                        };

                        match engine.evaluate_deviation(&payload, &sandbox) {
                            Ok(true) => {
                                let _ = ledger.commit_entry(&payload.agent_id, &payload.ticker, "REJECTED_DIVERGENCE", &line);
                                let _ = KillSignalPayload::dispatch(
                                    &mut write_half,
                                    payload.agent_id,
                                    payload.ticker,
                                    format!("Slippage threshold exceeded (Delta > {:.1}bps)", slippage * 10000.0),
                                ).await;
                            }
                            Err(e) => {
                                eprintln!("Divergence evaluation error: {}", e);
                            }
                            Ok(false) => {
                                let _ = ledger.commit_entry(&payload.agent_id, &payload.ticker, "APPROVED", &line);
                            }
                        }
                    }
                }
            });
        }
        Ok(())
    }
}

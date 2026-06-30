use serde::Serialize;
use tokio::io::AsyncWriteExt;

#[derive(Serialize, Debug)]
pub struct KillSignalPayload {
    pub signal_type: &'static str,
    pub agent_id: String,
    pub ticker: String,
    pub violation_reason: String,
    pub timestamp: i64,
}

impl KillSignalPayload {
    pub async fn dispatch(
        stream: &mut tokio::net::unix::OwnedWriteHalf,
        agent_id: String,
        ticker: String,
        reason: String,
    ) -> Result<(), std::io::Error> {
        let signal = KillSignalPayload {
            signal_type: "KILL_FLATTEN",
            agent_id,
            ticker,
            violation_reason: reason,
            timestamp: chrono::Utc::now().timestamp_nanos_opt().unwrap_or(0),
        };

        if let Ok(raw_json) = serde_json::to_vec(&signal) {
            let mut packet = raw_json;
            packet.push(b'\n');

            stream.write_all(&packet).await?;
            stream.flush().await?;
        }
        Ok(())
    }
}

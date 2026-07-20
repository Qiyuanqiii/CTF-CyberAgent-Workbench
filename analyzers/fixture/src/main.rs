use std::process::ExitCode;

use clap::Parser;

#[derive(Parser)]
#[command(
    name = "cyberagent-analyzer-fixture",
    version,
    about = "Deterministic analyzer protocol fixture; reads stdin JSON and writes stdout JSON"
)]
struct Cli {}

fn main() -> ExitCode {
    let _ = Cli::parse();
    match cyberagent_analyzer_fixture::run_io(std::io::stdin().lock(), std::io::stdout().lock()) {
        Ok(code) => ExitCode::from(code),
        Err(_) => {
            let _ = std::io::Write::write_all(
                &mut std::io::stdout().lock(),
                &cyberagent_analyzer_fixture::internal_error(),
            );
            ExitCode::from(cyberagent_analyzer_fixture::EXIT_INTERNAL)
        }
    }
}

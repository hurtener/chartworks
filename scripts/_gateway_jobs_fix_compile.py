from pathlib import Path
p=Path('internal/auth/verifier.go');s=p.read_text().replace('\n\tExecution\n','\n\tExecutionSurface\n').replace('case Execution:','case ExecutionSurface:');p.write_text(s)
p=Path('internal/auth/execution.go');s=p.read_text().replace('v.Verify(ctx, token, Execution)','v.Verify(ctx, token, ExecutionSurface)');p.write_text(s)

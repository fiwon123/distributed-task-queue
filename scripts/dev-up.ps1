param([string]$Command="help",[string[]]$Args)
$py = Get-Command python -ErrorAction SilentlyContinue; if(-not $py){$py=Get-Command py -ErrorAction SilentlyContinue}
Set-Location (Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path))
& $py.Source scripts/dev-up.py $Command @Args; exit $LASTEXITCODE

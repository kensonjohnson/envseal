package cli

// supportedCompletionShell reports whether shell is a completion render target.
func supportedCompletionShell(shell string) bool {
	switch shell {
	case "bash", "zsh", "fish", "powershell":
		return true
	default:
		return false
	}
}

// completionScript returns a static script for a validated shell target.
func completionScript(shell string) string {
	switch shell {
	case "bash":
		return bashCompletion
	case "zsh":
		return zshCompletion
	case "fish":
		return fishCompletion
	case "powershell":
		return powershellCompletion
	default:
		return ""
	}
}

const bashCompletion = `# Envseal Bash completion.
# This also supports the primary "go tool envseal" workflow.
if ! command -v envseal >/dev/null 2>&1; then
  envseal() { go tool envseal "$@"; }
fi

_envseal_complete() {
  local cur command word mode
  local index positional after_options dry_run expects_value
  cur=${COMP_WORDS[COMP_CWORD]}
  COMPREPLY=()

  if (( COMP_CWORD == 1 )); then
    COMPREPLY=( $(compgen -W 'encrypt decrypt rotate check generate completion --help --version' -- "$cur") )
    return
  fi

  command=${COMP_WORDS[1]}
  positional=0
  after_options=0
  dry_run=0
  expects_value=0
  for ((index = 2; index < COMP_CWORD; index++)); do
    word=${COMP_WORDS[index]}
    if (( ! after_options )) && [[ "$word" == '--' ]]; then
      after_options=1
      continue
    fi
    if (( ! after_options )); then
      case "$word" in
        --quiet|--force|--help) continue ;;
        --dry-run) dry_run=1; continue ;;
        --words|--bytes)
          if (( index + 1 == COMP_CWORD )); then expects_value=1; break; fi
          ((index++))
          continue
          ;;
      esac
    fi
    ((positional++))
    if [[ "$command" == generate ]] && (( positional == 1 )); then mode=$word; fi
  done

  if (( expects_value )); then return; fi

  if (( ! after_options )) && [[ "$cur" == -* ]]; then
    case "$command" in
      encrypt|rotate|check) COMPREPLY=( $(compgen -W '--quiet --help' -- "$cur") ) ;;
      decrypt) COMPREPLY=( $(compgen -W '--quiet --force --dry-run --help' -- "$cur") ) ;;
      generate)
        case "$mode" in
          passphrase) COMPREPLY=( $(compgen -W '--words --help' -- "$cur") ) ;;
          secret) COMPREPLY=( $(compgen -W '--bytes --help' -- "$cur") ) ;;
          *) COMPREPLY=( $(compgen -W '--words --bytes --help' -- "$cur") ) ;;
        esac
        ;;
      completion)
        if [[ ${COMP_WORDS[2]} == install ]]; then
          COMPREPLY=( $(compgen -W '--configure-shell --help' -- "$cur") )
        else
          COMPREPLY=( $(compgen -W '--help' -- "$cur") )
        fi
        ;;
    esac
    return
  fi

  case "$command" in
    encrypt)
      if (( positional == 0 )); then COMPREPLY=( $(compgen -f -- "$cur") ); fi
      ;;
    decrypt)
      if (( ! dry_run && positional < 2 )) || (( dry_run && positional == 0 )); then
        COMPREPLY=( $(compgen -f -- "$cur") )
      fi
      ;;
    rotate|check)
      if (( positional == 0 )); then COMPREPLY=( $(compgen -f -- "$cur") ); fi
      ;;
    generate)
      if (( positional == 0 )); then COMPREPLY=( $(compgen -W 'passphrase secret' -- "$cur") ); fi
      ;;
    completion)
      if (( positional == 0 )); then
        COMPREPLY=( $(compgen -W 'bash zsh fish powershell install' -- "$cur") )
      elif [[ ${COMP_WORDS[2]} == install ]] && (( positional == 1 )); then
        COMPREPLY=( $(compgen -W 'bash zsh fish powershell' -- "$cur") )
      fi
      ;;
  esac
}
complete -F _envseal_complete envseal
`

const zshCompletion = `#compdef envseal
# Envseal Zsh completion. This also supports the primary "go tool envseal" workflow.
if (( ! $+commands[envseal] && ! $+functions[envseal] )); then
  envseal() { go tool envseal "$@"; }
fi

_envseal() {
  local state
  local -a commands
  commands=(
    'encrypt:encrypt selected values in a file'
    'decrypt:decrypt a file to an explicit output'
    'rotate:re-encrypt a file with a new password'
    'check:validate a file'
    'generate:generate a credential'
    'completion:render or install shell completion'
  )

  if (( CURRENT == 2 )); then
    _describe 'command' commands
    _values 'global option' '--help[Show root help]' '--version[Print the build version]'
    return
  fi

  case $words[2] in
    encrypt)
      _arguments '--quiet[Suppress the success summary]' '--help[Show help]' '1:source file:_files' '*:key:'
      ;;
    decrypt)
      if (( ${words[(I)--dry-run]} )); then
        _arguments '--quiet[Suppress the success summary]' '--dry-run[Authenticate without writing]' '--help[Show help]' '1:source file:_files'
      else
        _arguments '--quiet[Suppress the success summary]' '--force[Replace an existing output]' '--dry-run[Authenticate without writing]' '--help[Show help]' '1:source file:_files' '2:plaintext output:_files'
      fi
      ;;
    rotate|check)
      _arguments '--quiet[Suppress the success summary]' '--help[Show help]' '1:source file:_files'
      ;;
    generate)
      if (( ${words[(I)passphrase]} )); then
        _arguments '--words[Passphrase word count]:count:' '--help[Show help]'
      elif (( ${words[(I)secret]} )); then
        _arguments '--bytes[Secret byte count]:count:' '--help[Show help]'
      else
        _arguments '--words[Passphrase word count]:count:' '--bytes[Secret byte count]:count:' '--help[Show help]' '1:mode:(passphrase secret)'
      fi
      ;;
    completion)
      if [[ $words[3] == install ]]; then
        _arguments '--configure-shell[Allow a shell startup-file edit]' '--help[Show help]' '1:action:(install)' '2:shell:(bash zsh fish powershell)'
      else
        _arguments '--help[Show help]' '1:target:(bash zsh fish powershell install)'
      fi
      ;;
  esac
}
compdef _envseal envseal
`

const fishCompletion = `# Envseal Fish completion.
# This also supports the primary "go tool envseal" workflow.
if not type -q envseal
    function envseal
        go tool envseal $argv
    end
end

function __fish_envseal_file_position
    set -l words (commandline -opc)
    if test (count $words) -lt 2
        return 1
    end
    set -e words[1]
    set -l command $words[1]
    set -e words[1]
    set -l positional 0
    set -l after_options 0
    set -l skip_value 0
    set -l dry_run 0
    for word in $words
        if test $skip_value -eq 1
            set skip_value 0
            continue
        end
        if test $after_options -eq 0
            switch $word
                case --
                    set after_options 1
                    continue
                case --quiet --force --help
                    continue
                case --dry-run
                    set dry_run 1
                    continue
                case --words --bytes
                    set skip_value 1
                    continue
            end
        end
        set positional (math $positional + 1)
    end
    switch $command
        case encrypt rotate check
            test $positional -eq 0
        case decrypt
            if test $dry_run -eq 1
                test $positional -eq 0
            else
                test $positional -lt 2
            end
        case '*'
            return 1
    end
end

function __fish_envseal_command_is
    set -l words (commandline -opc)
    test (count $words) -ge 2; and test $words[2] = $argv[1]
end

function __fish_envseal_completion_position
    set -l expected $argv[1]
    set -l words (commandline -opc)
    if test (count $words) -lt 2
        return 1
    end
    set -e words[1]
    if test $words[1] != completion
        return 1
    end
    set -e words[1]
    set -l positional 0
    set -l after_options 0
    for word in $words
        if test $after_options -eq 0
            switch $word
                case --
                    set after_options 1
                    continue
                case --help --configure-shell
                    continue
            end
        end
        set positional (math $positional + 1)
    end
    test $positional -eq $expected
end

function __fish_envseal_completion_install
    set -l words (commandline -opc)
    test (count $words) -ge 3; and test $words[2] = completion; and test $words[3] = install
end

complete -c envseal -f
complete -c envseal -n '__fish_use_subcommand' -a 'encrypt decrypt rotate check generate completion'
complete -c envseal -n '__fish_use_subcommand' -l help -d 'Show help'
complete -c envseal -n '__fish_use_subcommand' -l version -d 'Print the build version'
complete -c envseal -n '__fish_seen_subcommand_from encrypt rotate check; and not __fish_seen_argument -l quiet' -l quiet -d 'Suppress the success summary'
complete -c envseal -n '__fish_seen_subcommand_from decrypt; and not __fish_seen_argument -l quiet' -l quiet -d 'Suppress the success summary'
complete -c envseal -n '__fish_seen_subcommand_from decrypt; and not __fish_seen_argument -l force' -l force -d 'Replace an existing output'
complete -c envseal -n '__fish_seen_subcommand_from decrypt; and not __fish_seen_argument -l dry-run' -l dry-run -d 'Authenticate without writing'
complete -c envseal -n '__fish_seen_subcommand_from encrypt decrypt rotate check generate completion; and not __fish_seen_argument -l help' -l help -d 'Show help'
complete -c envseal -n '__fish_envseal_command_is generate; and not __fish_seen_subcommand_from passphrase secret' -a 'passphrase secret'
complete -c envseal -n '__fish_envseal_command_is generate; and not __fish_seen_subcommand_from secret; and not __fish_seen_argument -l words' -l words -r -d 'Passphrase word count'
complete -c envseal -n '__fish_envseal_command_is generate; and not __fish_seen_subcommand_from passphrase; and not __fish_seen_argument -l bytes' -l bytes -r -d 'Secret byte count'
complete -c envseal -n '__fish_envseal_completion_position 0' -a 'bash zsh fish powershell install'
complete -c envseal -n '__fish_envseal_completion_install; and __fish_envseal_completion_position 1' -a 'bash zsh fish powershell'
complete -c envseal -n '__fish_envseal_completion_install; and not __fish_seen_argument -l configure-shell' -l configure-shell -d 'Allow a shell startup-file edit'
complete -c envseal -n '__fish_envseal_file_position' -F
`

const powershellCompletion = `# Envseal PowerShell completion.
# This also supports the primary "go tool envseal" workflow.
if (-not (Get-Command envseal -ErrorAction SilentlyContinue)) {
    function global:envseal { & go tool envseal @args }
}

Register-ArgumentCompleter -Native -CommandName envseal -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)

    $words = @($commandAst.CommandElements | ForEach-Object { $_.Extent.Text })
    $complete = {
        param([string[]]$values)
        $values |
            Where-Object { $_ -like "$wordToComplete*" } |
            ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) }
    }
    $quoteCompletion = {
        param([string]$value)
        "'" + $value.Replace("'", "''") + "'"
    }
    $completeFiles = {
        Get-ChildItem -Path ($wordToComplete + '*') -Force -ErrorAction SilentlyContinue |
            ForEach-Object {
                $insertionText = & $quoteCompletion $_.FullName
                [System.Management.Automation.CompletionResult]::new($insertionText, $_.Name, 'ProviderItem', $_.FullName)
            }
    }

    if ($words.Count -le 1) {
        & $complete @('encrypt', 'decrypt', 'rotate', 'check', 'generate', 'completion', '--help', '--version')
        return
    }

    $command = $words[1]
    $arguments = @($words | Select-Object -Skip 2)
    if ($wordToComplete -and $arguments.Count -gt 0 -and $arguments[-1] -eq $wordToComplete) {
        $arguments = @($arguments | Select-Object -First ($arguments.Count - 1))
    }

    $positionals = @()
    $afterOptions = $false
    $dryRun = $false
    $expectsOptionValue = $false
    for ($index = 0; $index -lt $arguments.Count; $index++) {
        $argument = $arguments[$index]
        $consumedOption = $false
        if (-not $afterOptions) {
            switch ($argument) {
                '--' { $afterOptions = $true; $consumedOption = $true }
                '--quiet' { $consumedOption = $true }
                '--force' { $consumedOption = $true }
                '--dry-run' { $dryRun = $true; $consumedOption = $true }
                '--help' { $consumedOption = $true }
                '--configure-shell' { $consumedOption = $true }
                '--words' {
                    if ($index + 1 -eq $arguments.Count) { $expectsOptionValue = $true }
                    else { $index++ }
                    $consumedOption = $true
                }
                '--bytes' {
                    if ($index + 1 -eq $arguments.Count) { $expectsOptionValue = $true }
                    else { $index++ }
                    $consumedOption = $true
                }
            }
        }
        if ($consumedOption) { continue }
        $positionals += $argument
    }

    if ($expectsOptionValue) { return }

    if (-not $afterOptions -and $wordToComplete -like '-*') {
        switch ($command) {
            'encrypt' { & $complete @('--quiet', '--help') }
            'decrypt' { & $complete @('--quiet', '--force', '--dry-run', '--help') }
            'rotate' { & $complete @('--quiet', '--help') }
            'check' { & $complete @('--quiet', '--help') }
            'generate' {
                if ($positionals -contains 'passphrase') { & $complete @('--words', '--help') }
                elseif ($positionals -contains 'secret') { & $complete @('--bytes', '--help') }
                else { & $complete @('--words', '--bytes', '--help') }
            }
            'completion' {
                if ($positionals.Count -gt 0 -and $positionals[0] -eq 'install') { & $complete @('--configure-shell', '--help') }
                else { & $complete @('--help') }
            }
        }
        return
    }

    switch ($command) {
        'encrypt' { if ($positionals.Count -eq 0) { & $completeFiles } }
        'decrypt' {
            if ($dryRun) {
                if ($positionals.Count -eq 0) { & $completeFiles }
            } elseif ($positionals.Count -lt 2) {
                & $completeFiles
            }
        }
        'rotate' { if ($positionals.Count -eq 0) { & $completeFiles } }
        'check' { if ($positionals.Count -eq 0) { & $completeFiles } }
        'generate' { if ($positionals.Count -eq 0) { & $complete @('passphrase', 'secret') } }
        'completion' {
            if ($positionals.Count -eq 0) { & $complete @('bash', 'zsh', 'fish', 'powershell', 'install') }
            elseif ($positionals.Count -eq 1 -and $positionals[0] -eq 'install') { & $complete @('bash', 'zsh', 'fish', 'powershell') }
        }
    }
}
`

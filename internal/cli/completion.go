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
  local index positional after_options
  cur=${COMP_WORDS[COMP_CWORD]}
  COMPREPLY=()

  if (( COMP_CWORD == 1 )); then
    COMPREPLY=( $(compgen -W 'encrypt decrypt rotate check generate completion --help --version' -- "$cur") )
    return
  fi

  command=${COMP_WORDS[1]}
  if [[ "$cur" == -* ]]; then
    case "$command" in
      encrypt|rotate|check) COMPREPLY=( $(compgen -W '--quiet --help' -- "$cur") ) ;;
      decrypt) COMPREPLY=( $(compgen -W '--quiet --force --dry-run --help' -- "$cur") ) ;;
      generate)
        for ((index = 2; index < COMP_CWORD; index++)); do
          case ${COMP_WORDS[index]} in
            passphrase|secret) mode=${COMP_WORDS[index]} ;;
          esac
        done
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

  positional=0
  after_options=0
  for ((index = 2; index < COMP_CWORD; index++)); do
    word=${COMP_WORDS[index]}
    if (( ! after_options )) && [[ "$word" == '--' ]]; then
      after_options=1
      continue
    fi
    if (( ! after_options )); then
      case "$word" in
        --quiet|--force|--dry-run|--help) continue ;;
        --words|--bytes) ((index++)); continue ;;
      esac
    fi
    ((positional++))
  done

  case "$command" in
    encrypt)
      if (( positional == 0 )); then COMPREPLY=( $(compgen -f -- "$cur") ); fi
      ;;
    decrypt)
      if [[ " ${COMP_WORDS[*]} " != *' --dry-run '* ]] && (( positional < 2 )); then
        COMPREPLY=( $(compgen -f -- "$cur") )
      elif [[ " ${COMP_WORDS[*]} " == *' --dry-run '* ]] && (( positional == 0 )); then
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
      _arguments '--quiet[Suppress the success summary]' '--force[Replace an existing output]' '--dry-run[Authenticate without writing]' '--help[Show help]' '1:source file:_files' '2:plaintext output:_files'
      ;;
    rotate|check)
      _arguments '--quiet[Suppress the success summary]' '--help[Show help]' '1:source file:_files'
      ;;
    generate)
      if (( ${words[(I)passphrase]} )); then
        _arguments '--words=[Passphrase word count]:count:' '--help[Show help]'
      elif (( ${words[(I)secret]} )); then
        _arguments '--bytes=[Secret byte count]:count:' '--help[Show help]'
      else
        _arguments '--help[Show help]' '1:mode:(passphrase secret)'
      fi
      ;;
    completion)
      if [[ $words[3] == install ]]; then
        _arguments '--configure-shell[Allow a shell startup-file edit]' '--help[Show help]' '1:action:(install)' '2:shell:(bash zsh fish powershell)'
      else
        _arguments '--help[Show help]' '1:target:(bash zsh fish powershell install)' '2:shell:(bash zsh fish powershell)'
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
    for word in $words
        switch $word
            case --quiet --force --dry-run --help
                continue
        end
        set positional (math $positional + 1)
    end
    switch $command
        case encrypt rotate check
            test $positional -eq 0
        case decrypt
            if contains -- --dry-run $words
                test $positional -eq 0
            else
                test $positional -lt 2
            end
        case '*'
            return 1
    end
end

complete -c envseal -f
complete -c envseal -n '__fish_use_subcommand' -a 'encrypt decrypt rotate check generate completion'
complete -c envseal -n '__fish_use_subcommand' -l help -d 'Show help'
complete -c envseal -n '__fish_use_subcommand' -l version -d 'Print the build version'
complete -c envseal -n '__fish_seen_subcommand_from encrypt rotate check' -l quiet -d 'Suppress the success summary'
complete -c envseal -n '__fish_seen_subcommand_from decrypt' -l quiet -d 'Suppress the success summary'
complete -c envseal -n '__fish_seen_subcommand_from decrypt' -l force -d 'Replace an existing output'
complete -c envseal -n '__fish_seen_subcommand_from decrypt' -l dry-run -d 'Authenticate without writing'
complete -c envseal -n '__fish_seen_subcommand_from encrypt decrypt rotate check generate completion' -l help -d 'Show help'
complete -c envseal -n '__fish_seen_subcommand_from generate; and not __fish_seen_subcommand_from passphrase secret' -a 'passphrase secret'
complete -c envseal -n '__fish_seen_subcommand_from passphrase' -l words -r -d 'Passphrase word count'
complete -c envseal -n '__fish_seen_subcommand_from secret' -l bytes -r -d 'Secret byte count'
complete -c envseal -n '__fish_seen_subcommand_from completion; and not __fish_seen_subcommand_from install' -a 'bash zsh fish powershell install'
complete -c envseal -n '__fish_seen_subcommand_from completion install' -a 'bash zsh fish powershell'
complete -c envseal -n '__fish_seen_subcommand_from completion install' -l configure-shell -d 'Allow a shell startup-file edit'
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
    $completeFiles = {
        Get-ChildItem -Path ($wordToComplete + '*') -Force -ErrorAction SilentlyContinue |
            ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_.FullName, $_.Name, 'ProviderItem', $_.FullName)
            }
    }

    if ($words.Count -le 1) {
        & $complete @('encrypt', 'decrypt', 'rotate', 'check', 'generate', 'completion', '--help', '--version')
        return
    }

    $command = $words[1]
    if ($wordToComplete -like '-*') {
        switch ($command) {
            'encrypt' { & $complete @('--quiet', '--help') }
            'decrypt' { & $complete @('--quiet', '--force', '--dry-run', '--help') }
            'rotate' { & $complete @('--quiet', '--help') }
            'check' { & $complete @('--quiet', '--help') }
            'generate' {
                if ($words -contains 'passphrase') { & $complete @('--words', '--help') }
                elseif ($words -contains 'secret') { & $complete @('--bytes', '--help') }
                else { & $complete @('--words', '--bytes', '--help') }
            }
            'completion' {
                if ($words -contains 'install') { & $complete @('--configure-shell', '--help') }
                else { & $complete @('--help') }
            }
        }
        return
    }

    $arguments = @($words | Select-Object -Skip 2)
    if ($wordToComplete -and $arguments.Count -gt 0 -and $arguments[-1] -eq $wordToComplete) {
        $arguments = @($arguments | Select-Object -First ($arguments.Count - 1))
    }
    $positionals = @($arguments | Where-Object { $_ -notlike '-*' })
    switch ($command) {
        'encrypt' { if ($positionals.Count -eq 0) { & $completeFiles } }
        'decrypt' {
            if ($words -contains '--dry-run') {
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

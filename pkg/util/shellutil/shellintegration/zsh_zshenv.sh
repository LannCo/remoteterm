# Store the initial ZDOTDIR value
REMOTETERM_ZDOTDIR="$ZDOTDIR"

# Source the original zshenv
[ -f ~/.zshenv ] && source ~/.zshenv

# Detect if ZDOTDIR has changed
if [ "$ZDOTDIR" != "$REMOTETERM_ZDOTDIR" ]; then
  # If changed, manually source your custom zshrc from the original REMOTETERM_ZDOTDIR
  [ -f "$REMOTETERM_ZDOTDIR/.zshrc" ] && source "$REMOTETERM_ZDOTDIR/.zshrc"
fi
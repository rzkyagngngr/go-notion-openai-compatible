package browserrefresh

func MaskToken(token string) string {
	if token == "" {
		return "••••"
	}
	if len(token) <= 12 {
		return "••••"
	}
	return token[:6] + "••••" + token[len(token)-4:]
}
package i18n

// messagesPT contains the built-in Portuguese translations.
// Registered under both "pt" and "pt-BR" by NewDefault.
var messagesPT = Messages{
	// ── Erros de status HTTP ──────────────────────────────────────────────────
	"http.400": "Requisição inválida",
	"http.401": "Não autorizado",
	"http.403": "Acesso proibido",
	"http.404": "Recurso não encontrado",
	"http.405": "Método não permitido",
	"http.409": "Conflito de dados, tente novamente",
	"http.422": "Formato de dados inválido",
	"http.429": "Muitas requisições, tente novamente mais tarde",
	"http.500": "Erro interno do servidor",
	"http.503": "Serviço temporariamente indisponível",

	// ── Erros de validação por campo ──────────────────────────────────────────
	"validate.required":  "%s é obrigatório",
	"validate.min":       "%s deve ter pelo menos %s caracteres",
	"validate.max":       "%s deve ter no máximo %s caracteres",
	"validate.len":       "%s deve ter exatamente %s caracteres",
	"validate.gte":       "%s deve ser maior ou igual a %s",
	"validate.lte":       "%s deve ser menor ou igual a %s",
	"validate.gt":        "%s deve ser maior que %s",
	"validate.lt":        "%s deve ser menor que %s",
	"validate.email":     "%s deve ser um endereço de e-mail válido",
	"validate.url":       "%s deve ser uma URL válida",
	"validate.numeric":   "%s deve ser um número",
	"validate.alpha":     "%s deve conter apenas letras",
	"validate.alphanum":  "%s deve conter apenas letras e números",
	"validate.oneof":     "%s deve ser um dos seguintes: %s",
	"validate.unique":    "%s não deve conter valores duplicados",
	"validate.mobile":    "%s deve ser um número de celular válido",
	"validate.password":  "%s deve ter pelo menos 8 caracteres e incluir maiúsculas, minúsculas, dígitos e caracteres especiais",
	"validate.username":  "%s deve ter entre 3 e 32 caracteres e conter apenas letras, dígitos e sublinhados",
	"validate.no_html":   "%s não deve conter tags HTML",
	"validate.not_blank": "%s não deve estar em branco",

	// ── Mensagens de negócio comuns ───────────────────────────────────────────
	"common.success":          "Sucesso",
	"common.created":          "Criado com sucesso",
	"common.updated":          "Atualizado com sucesso",
	"common.deleted":          "Excluído com sucesso",
	"common.not_found":        "Recurso não encontrado",
	"common.unauthorized":     "Por favor, faça login para continuar",
	"common.forbidden":        "Você não tem permissão para realizar esta ação",
	"common.invalid_params":   "Parâmetros de requisição inválidos",
	"common.server_error":     "Ocorreu um erro interno. Tente novamente mais tarde.",
	"common.rate_limited":     "Muitas requisições. Por favor, aguarde um momento.",
	"common.token_expired":    "Sessão expirada. Por favor, faça login novamente.",
	"common.token_invalid":    "Token de autenticação inválido ou ausente",
	"common.bad_content":      "Tipo de conteúdo não suportado",
	"common.upload_too_large": "O arquivo é muito grande",
	"common.maintenance":      "O serviço está em manutenção. Tente novamente mais tarde.",
}

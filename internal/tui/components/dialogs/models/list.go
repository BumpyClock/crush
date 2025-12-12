package models

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/catwalk/pkg/catwalk"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/tui/exp/list"
	"github.com/charmbracelet/crush/internal/tui/styles"
	"github.com/charmbracelet/crush/internal/tui/util"
)

type listModel = list.FilterableGroupList[list.CompletionItem[ModelOption]]

type ModelListComponent struct {
	list      listModel
	modelType int
	providers []catwalk.Provider
}

func modelKey(providerID, modelID string) string {
	if providerID == "" || modelID == "" {
		return ""
	}
	return providerID + ":" + modelID
}

func NewModelListComponent(keyMap list.KeyMap, inputPlaceholder string, shouldResize bool) *ModelListComponent {
	t := styles.CurrentTheme()
	inputStyle := t.S().Base.PaddingLeft(1).PaddingBottom(1)
	options := []list.ListOption{
		list.WithKeyMap(keyMap),
		list.WithWrapNavigation(),
	}
	if shouldResize {
		options = append(options, list.WithResizeByList())
	}
	modelList := list.NewFilterableGroupedList(
		[]list.Group[list.CompletionItem[ModelOption]]{},
		list.WithFilterInputStyle(inputStyle),
		list.WithFilterPlaceholder(inputPlaceholder),
		list.WithFilterListOptions(
			options...,
		),
	)

	return &ModelListComponent{
		list:      modelList,
		modelType: LargeModelType,
	}
}

func (m *ModelListComponent) Init() tea.Cmd {
	var cmds []tea.Cmd
	if len(m.providers) == 0 {
		cfg := config.Get()
		providers, err := config.Providers(cfg)
		filteredProviders := make([]catwalk.Provider, 0, len(providers))
		for _, p := range providers {
			hasAPIKeyEnv := strings.HasPrefix(p.APIKey, "$")
			isOAuthProvider := config.IsOAuthProvider(string(p.ID))
			// Skip OAuth providers in test environments (when disable_provider_auto_update is true)
			if isOAuthProvider && cfg.Options.DisableProviderAutoUpdate {
				continue
			}
			// Include providers with API key env vars or OAuth providers
			if (hasAPIKeyEnv && p.ID != catwalk.InferenceProviderAzure) || isOAuthProvider {
				filteredProviders = append(filteredProviders, p)
			}
		}

		m.providers = filteredProviders
		if err != nil {
			cmds = append(cmds, util.ReportError(err))
		}
	}
	cmds = append(cmds, m.list.Init(), m.SetModelType(m.modelType))
	return tea.Batch(cmds...)
}

func (m *ModelListComponent) Update(msg tea.Msg) (*ModelListComponent, tea.Cmd) {
	u, cmd := m.list.Update(msg)
	m.list = u.(listModel)
	return m, cmd
}

func (m *ModelListComponent) View() string {
	return m.list.View()
}

func (m *ModelListComponent) Cursor() *tea.Cursor {
	return m.list.Cursor()
}

func (m *ModelListComponent) SetSize(width, height int) tea.Cmd {
	return m.list.SetSize(width, height)
}

func (m *ModelListComponent) SelectedModel() *ModelOption {
	s := m.list.SelectedItem()
	if s == nil {
		return nil
	}
	sv := *s
	model := sv.Value()
	return &model
}

func (m *ModelListComponent) SetModelType(modelType int) tea.Cmd {
	t := styles.CurrentTheme()
	m.modelType = modelType

	var groups []list.Group[list.CompletionItem[ModelOption]]
	selectedItemID := ""
	itemsByKey := make(map[string]list.CompletionItem[ModelOption])

	cfg := config.Get()
	var currentModel config.SelectedModel
	selectedType := config.SelectedModelTypeLarge
	if m.modelType == LargeModelType {
		currentModel = cfg.Models[config.SelectedModelTypeLarge]
		selectedType = config.SelectedModelTypeLarge
	} else {
		currentModel = cfg.Models[config.SelectedModelTypeSmall]
		selectedType = config.SelectedModelTypeSmall
	}
	recentItems := cfg.RecentModels[selectedType]

	configuredIcon := t.S().Base.Foreground(t.Success).Render(styles.CheckIcon)
	configured := fmt.Sprintf("%s %s", configuredIcon, t.S().Subtle.Render("Configured"))

	addedProviders := make(map[string]bool)

	prepareProvider := func(provider catwalk.Provider) catwalk.Provider {
		displayProvider := provider
		if providerConfig, providerConfigured := cfg.Providers.Get(string(provider.ID)); providerConfigured {
			displayProvider.Name = cmp.Or(providerConfig.Name, displayProvider.Name)
			modelIndex := make(map[string]int, len(displayProvider.Models))
			for i, model := range displayProvider.Models {
				modelIndex[model.ID] = i
			}
			for _, model := range providerConfig.Models {
				if model.ID == "" {
					continue
				}
				if idx, ok := modelIndex[model.ID]; ok {
					if model.Name != "" {
						displayProvider.Models[idx].Name = model.Name
					}
					continue
				}
				if model.Name == "" {
					model.Name = model.ID
				}
				displayProvider.Models = append(displayProvider.Models, model)
				modelIndex[model.ID] = len(displayProvider.Models) - 1
			}
		}
		return displayProvider
	}

	knownProviders, err := config.Providers(cfg)
	if err != nil {
		return util.ReportError(err)
	}
	for providerID, providerConfig := range cfg.Providers.Seq2() {
		if providerConfig.Disable {
			continue
		}

		if !slices.ContainsFunc(knownProviders, func(p catwalk.Provider) bool { return p.ID == catwalk.InferenceProvider(providerID) }) ||
			!slices.ContainsFunc(m.providers, func(p catwalk.Provider) bool { return p.ID == catwalk.InferenceProvider(providerID) }) {
			configProvider := catwalk.Provider{
				Name:   providerConfig.Name,
				ID:     catwalk.InferenceProvider(providerID),
				Models: make([]catwalk.Model, len(providerConfig.Models)),
			}

			for i, model := range providerConfig.Models {
				configProvider.Models[i] = catwalk.Model{
					ID:                     model.ID,
					Name:                   model.Name,
					CostPer1MIn:            model.CostPer1MIn,
					CostPer1MOut:           model.CostPer1MOut,
					CostPer1MInCached:      model.CostPer1MInCached,
					CostPer1MOutCached:     model.CostPer1MOutCached,
					ContextWindow:          model.ContextWindow,
					DefaultMaxTokens:       model.DefaultMaxTokens,
					CanReason:              model.CanReason,
					ReasoningLevels:        model.ReasoningLevels,
					DefaultReasoningEffort: model.DefaultReasoningEffort,
					SupportsImages:         model.SupportsImages,
				}
			}

			name := configProvider.Name
			if name == "" {
				name = string(configProvider.ID)
			}
			section := list.NewItemSection(name)
			section.SetInfo(configured)
			group := list.Group[list.CompletionItem[ModelOption]]{
				Section: section,
			}
			for _, model := range configProvider.Models {
				modelOption := ModelOption{
					Provider: configProvider,
					Model:    model,
				}
				key := modelKey(string(configProvider.ID), model.ID)
				item := list.NewCompletionItem(
					model.Name,
					modelOption,
					list.WithCompletionID(key),
				)
				itemsByKey[key] = item

				group.Items = append(group.Items, item)
				if model.ID == currentModel.Model && string(configProvider.ID) == currentModel.Provider {
					selectedItemID = item.ID()
				}
			}
			groups = append(groups, group)

			addedProviders[providerID] = true
		}
	}

	var priorityProviders, regularProviders []catwalk.Provider
	// Track which providers should contribute to itemsByKey for recent model validation
	validProviderIDs := make(map[string]bool)

	// Add configured providers to valid set
	for providerID := range cfg.Providers.Seq2() {
		validProviderIDs[providerID] = true
	}

	// Only load OAuth providers if provider auto-update is enabled
	// This respects test environments where disable_provider_auto_update is true
	if !cfg.Options.DisableProviderAutoUpdate {
		oauthProviders := config.GetOAuthProviders(config.GlobalDataDir())
		for _, oauthProvider := range oauthProviders {
			if addedProviders[oauthProvider.ID] {
				continue
			}
			displayProvider := prepareProvider(oauthProvider.ToDisplayProvider())
			isPriority := m.isProviderPriority(displayProvider, cfg)
			if isPriority {
				priorityProviders = append(priorityProviders, displayProvider)
				validProviderIDs[oauthProvider.ID] = true
			} else {
				regularProviders = append(regularProviders, displayProvider)
				// OAuth providers without credentials are NOT valid for recent model validation
			}
			addedProviders[oauthProvider.ID] = true
		}
	}

	for _, provider := range m.providers {
		if addedProviders[string(provider.ID)] {
			continue
		}

		displayProvider := prepareProvider(provider)
		isPriority := m.isProviderPriority(displayProvider, cfg)
		if isPriority {
			priorityProviders = append(priorityProviders, displayProvider)
			validProviderIDs[string(provider.ID)] = true
		} else {
			regularProviders = append(regularProviders, displayProvider)
			// Non-OAuth providers from m.providers are considered valid (they have API key env vars)
			if !config.IsOAuthProvider(string(provider.ID)) {
				validProviderIDs[string(provider.ID)] = true
			}
		}
		addedProviders[string(provider.ID)] = true
	}

	for _, provider := range priorityProviders {
		group := m.createProviderGroup(provider, cfg, configured, currentModel, &selectedItemID, itemsByKey, validProviderIDs)
		groups = append(groups, group)
	}

	for _, provider := range regularProviders {
		group := m.createProviderGroup(provider, cfg, configured, currentModel, &selectedItemID, itemsByKey, validProviderIDs)
		groups = append(groups, group)
	}

	if len(recentItems) > 0 {
		recentSection := list.NewItemSection("Recently used")
		recentGroup := list.Group[list.CompletionItem[ModelOption]]{
			Section: recentSection,
		}
		var validRecentItems []config.SelectedModel
		for _, recent := range recentItems {
			key := modelKey(recent.Provider, recent.Model)
			option, ok := itemsByKey[key]
			if !ok {
				continue
			}
			validRecentItems = append(validRecentItems, recent)
			recentID := fmt.Sprintf("recent::%s", key)
			modelOption := option.Value()
			providerName := modelOption.Provider.Name
			if providerName == "" {
				providerName = string(modelOption.Provider.ID)
			}
			item := list.NewCompletionItem(
				modelOption.Model.Name,
				option.Value(),
				list.WithCompletionID(recentID),
				list.WithCompletionShortcut(providerName),
			)
			recentGroup.Items = append(recentGroup.Items, item)
			if recent.Model == currentModel.Model && recent.Provider == currentModel.Provider {
				selectedItemID = recentID
			}
		}

		if len(validRecentItems) != len(recentItems) {
			if err := cfg.SetConfigField(fmt.Sprintf("recent_models.%s", selectedType), validRecentItems); err != nil {
				return util.ReportError(err)
			}
		}

		if len(recentGroup.Items) > 0 {
			groups = append([]list.Group[list.CompletionItem[ModelOption]]{recentGroup}, groups...)
		}
	}

	var cmds []tea.Cmd

	if cmd := m.list.SetGroups(groups); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if cmd := m.list.SetSelected(selectedItemID); cmd != nil {
		cmds = append(cmds, cmd)
	}

	return tea.Sequence(cmds...)
}

// GetModelType returns the current model type
func (m *ModelListComponent) GetModelType() int {
	return m.modelType
}

func (m *ModelListComponent) SetInputPlaceholder(placeholder string) {
	m.list.SetInputPlaceholder(placeholder)
}

func (m *ModelListComponent) isProviderPriority(provider catwalk.Provider, cfg *config.Config) bool {
	providerID := string(provider.ID)

	if config.IsOAuthProvider(providerID) {
		if oauthProvider, ok := config.GetOAuthProvider(providerID, config.GlobalDataDir()); ok {
			return oauthProvider.HasOAuthCredentials()
		}
	}

	if providerConfig, exists := cfg.Providers.Get(providerID); exists {
		return !providerConfig.Disable && providerConfig.APIKey != ""
	}

	return false
}

func (m *ModelListComponent) createProviderGroup(provider catwalk.Provider, cfg *config.Config, configured string, currentModel config.SelectedModel, selectedItemID *string, itemsByKey map[string]list.CompletionItem[ModelOption], validProviderIDs map[string]bool) list.Group[list.CompletionItem[ModelOption]] {
	name := provider.Name
	if name == "" {
		name = string(provider.ID)
	}

	section := list.NewItemSection(name)
	if _, ok := cfg.Providers.Get(string(provider.ID)); ok {
		section.SetInfo(configured)
	}

	// Check if this provider should contribute to itemsByKey for recent model validation
	providerID := string(provider.ID)
	shouldAddToItemsByKey := validProviderIDs[providerID]

	group := list.Group[list.CompletionItem[ModelOption]]{
		Section: section,
	}

	for _, model := range provider.Models {
		key := modelKey(string(provider.ID), model.ID)
		item := list.NewCompletionItem(model.Name, ModelOption{
			Provider: provider,
			Model:    model,
		},
			list.WithCompletionID(key),
		)
		// Only add to itemsByKey if provider is in the valid set
		// This ensures recent models are only validated against actually usable providers
		if shouldAddToItemsByKey {
			itemsByKey[key] = item
		}
		group.Items = append(group.Items, item)
		if model.ID == currentModel.Model && string(provider.ID) == currentModel.Provider {
			*selectedItemID = item.ID()
		}
	}

	return group
}

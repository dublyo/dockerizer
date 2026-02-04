package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/dublyo/dockerizer/internal/agent"
	"github.com/dublyo/dockerizer/internal/recipe"
	"github.com/spf13/cobra"
)

var recipeCmd = &cobra.Command{
	Use:   "recipe [name]",
	Short: "Run a predefined recipe workflow",
	Long: `Execute a predefined recipe workflow.

Built-in recipes:
  analyze        - Analyze a repository and report detected stack
  generate       - Generate Docker configuration
  build-and-test - Generate, build, and test
  full-deploy    - Complete deployment workflow

Examples:
  dockerizer recipe analyze --path ./my-project
  dockerizer recipe generate --path ./my-project
  dockerizer recipe build-and-test --path ./my-project --image-tag myapp:v1

Custom recipes from file:
  dockerizer recipe --file ./my-recipe.yaml`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRecipe,
}

var recipeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available recipes",
	RunE:  runRecipeList,
}

func init() {
	recipeCmd.Flags().String("file", "", "Path to custom recipe YAML file")
	recipeCmd.Flags().String("path", ".", "Path to the project")
	recipeCmd.Flags().String("image-tag", "app:latest", "Docker image tag")
	recipeCmd.Flags().StringToString("var", nil, "Set recipe variables (key=value)")

	recipeCmd.AddCommand(recipeListCmd)
	rootCmd.AddCommand(recipeCmd)
}

func runRecipe(cmd *cobra.Command, args []string) error {
	filePath, _ := cmd.Flags().GetString("file")
	projectPath, _ := cmd.Flags().GetString("path")
	imageTag, _ := cmd.Flags().GetString("image-tag")
	extraVars, _ := cmd.Flags().GetStringToString("var")

	var r *recipe.Recipe
	var err error

	if filePath != "" {
		// Load from file
		r, err = recipe.Load(filePath)
		if err != nil {
			return fmt.Errorf("failed to load recipe: %w", err)
		}
	} else if len(args) > 0 {
		// Load built-in recipe
		r, err = recipe.GetBuiltinRecipe(args[0])
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("specify a recipe name or --file")
	}

	printInfo("Running recipe: %s", r.Name)
	printVerbose("Description: %s", r.Description)

	// Create tool executor
	toolDispatcher := agent.NewToolDispatcher(projectPath)

	// Create executor
	executor := recipe.NewExecutor(&toolExecutorAdapter{td: toolDispatcher})

	// Set variables
	executor.SetVariable("path", projectPath)
	executor.SetVariable("image_tag", imageTag)
	for k, v := range extraVars {
		executor.SetVariable(k, v)
	}

	// Execute recipe
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	result, err := executor.Execute(ctx, r)
	if err != nil {
		return fmt.Errorf("recipe failed: %w", err)
	}

	// Print results
	printInfo("")
	for _, step := range result.Steps {
		if step.Success {
			printSuccess("%s: completed", step.Name)
		} else {
			printError("%s: failed - %v", step.Name, step.Error)
		}
	}

	if result.Success {
		printSuccess("Recipe completed successfully")
	} else {
		printError("Recipe failed")
	}

	return nil
}

func runRecipeList(cmd *cobra.Command, args []string) error {
	printInfo("Available built-in recipes:")
	printInfo("")

	for _, name := range recipe.ListBuiltinRecipes() {
		r, _ := recipe.GetBuiltinRecipe(name)
		printInfo("  %-15s - %s", name, r.Description)
	}

	return nil
}

// toolExecutorAdapter adapts ToolDispatcher to recipe.ToolExecutor
type toolExecutorAdapter struct {
	td *agent.ToolDispatcher
}

func (a *toolExecutorAdapter) Execute(ctx context.Context, tool string, args map[string]interface{}) (string, error) {
	return a.td.Execute(ctx, tool, args)
}

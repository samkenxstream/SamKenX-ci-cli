package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/CircleCI-Public/circleci-cli/client"
	"github.com/pkg/errors"

	"github.com/machinebox/graphql"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var orbPath string

type orbConfigResponse struct {
	OrbConfig struct {
		Valid      bool
		SourceYaml string
		OutputYaml string

		Errors []struct {
			Message string
		}
	}
}

func newOrbCommand() *cobra.Command {

	orbListCommand := &cobra.Command{
		Use:   "list",
		Short: "List orbs",
		RunE:  listOrbs,
	}

	orbValidateCommand := &cobra.Command{
		Use:   "validate",
		Short: "validate an orb.yml",
		RunE:  validateOrb,
	}

	orbCommand := &cobra.Command{
		Use:   "orb",
		Short: "Operate on orbs",
	}

	orbValidateCommand.PersistentFlags().StringVarP(&orbPath, "path", "p", "orb.yml", "path to orb file")
	orbCommand.AddCommand(orbListCommand)
	orbCommand.AddCommand(orbValidateCommand)

	return orbCommand
}

func listOrbs(cmd *cobra.Command, args []string) error {

	ctx := context.Background()

	// Define a structure that matches the result of the GQL
	// query, so that we can use mapstructure to convert from
	// nested maps to a strongly typed struct.
	type orbList struct {
		Orbs struct {
			TotalCount int
			Edges      []struct {
				Cursor string
				Node   struct {
					Name string
				}
			}
			PageInfo struct {
				HasNextPage bool
			}
		}
	}

	request := graphql.NewRequest(`
query ListOrbs ($after: String!) {
  orbs(first: 20, after: $after) {
	totalCount,
    edges {
      cursor,
      node {
        name
      }
    }
    pageInfo {
      hasNextPage
    }
  }
}
	`)

	client := client.NewClient(viper.GetString("endpoint"), Logger)

	var result orbList
	currentCursor := ""

	for {
		request.Var("after", currentCursor)
		err := client.Run(ctx, request, &result)

		if err != nil {
			return errors.Wrap(err, "GraphQL query failed")
		}

		// Debug logging of result fields.
		// Logger.Prettyify(result)

		for i := range result.Orbs.Edges {
			edge := result.Orbs.Edges[i]
			currentCursor = edge.Cursor
			Logger.Infof("Orb: %s\n", edge.Node.Name)
		}

		if !result.Orbs.PageInfo.HasNextPage {
			break
		}
	}
	return nil

}

func loadOrbYaml(path string) (string, error) {

	orb, err := ioutil.ReadFile(path)

	fmt.Fprintln(os.Stderr, "******************** orb.go")
	fmt.Fprintln(os.Stderr, path)
	fmt.Fprintln(os.Stderr, err)
	if err != nil {
		return "", errors.Wrapf(err, "Could not load orb file at %s", path)
	}

	return string(orb), nil
}


func (response orbConfigResponse) processErrors() error {
	var buffer bytes.Buffer

	buffer.WriteString("\n")
	for i := range response.OrbConfig.Errors {
		buffer.WriteString("-- ")
		buffer.WriteString(response.OrbConfig.Errors[i].Message)
		buffer.WriteString(",\n")
	}

	return errors.New(buffer.String())
}

func orbValidateQuery(ctx context.Context) (*orbConfigResponse, error) {

	query := `
		query ValidateOrb ($orb: String!) {
			orbConfig(orbYaml: $orb) {
				valid,
				errors { message },
				sourceYaml,
				outputYaml
			}
		}`

	orb, err := loadOrbYaml(orbPath)
	if err != nil {
		return nil, err
	}

	variables := map[string]string{
		"orb": orb,
	}

	var response orbConfigResponse
	err = queryAPI(ctx, query, variables, &response)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to validate orb")
	}

	return &response, nil
}

func validateOrb(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	response, err := orbValidateQuery(ctx)

	if err != nil {
		return err
	}

	if !response.OrbConfig.Valid {
		return response.processErrors()
	}

	Logger.Infof("Orb at %s is valid", orbPath)
	return nil
}

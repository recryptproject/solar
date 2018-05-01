package solar

import (
	"fmt"
	"log"
	"strings"

	"github.com/recryptproject/solar/contract"

	"github.com/pkg/errors"
)

type deployTarget struct {
	file string
	name string
}

func parseDeployTarget(target string) deployTarget {
	parts := strings.Split(target, ":")

	if len(parts) == 1 {
		return deployTarget{
			file: parts[0],
			name: parts[0],
		}
	}

	// deploy name by default is the file path relative to project root
	return deployTarget{
		file: parts[0],
		name: parts[1],
	}
}

func init() {
	cmd := app.Command("deploy", "Compile Solidity contracts.")

	force := cmd.Flag("force", "Overwrite previously deployed contract with the same deploy name").Bool()
	aslib := cmd.Flag("lib", "Deploy the contract as a library").Bool()
	noconfirm := cmd.Flag("no-confirm", "Don't wait for network to confirm deploy").Bool()
	noFastConfirm := cmd.Flag("no-fast-confirm", "(dev) Don't generate block to confirm deploy immediately").Bool()
	gasLimit := cmd.Flag("gasLimit", "gas limit for creating a contract").Default("3000000").Int()

	target := cmd.Arg("target", "Solidity contracts to deploy.").Required().String()
	jsonParams := cmd.Arg("jsonParams", "Constructor params as a json array").Default("").String()

	appTasks["deploy"] = func() (err error) {
		target := parseDeployTarget(*target)

		opts, err := solar.SolcOptions()
		if err != nil {
			return
		}

		filename := target.file

		repo := solar.ContractsRepository()

		compiler := Compiler{
			Opts:     *opts,
			Filename: filename,
			Repo:     repo,
		}

		compiledContract, err := compiler.Compile()
		if err != nil {
			return errors.Wrap(err, "compile")
		}

		deployer := solar.Deployer()

		var params []byte
		if jsonParams != nil {
			jsonParams := solar.ExpandJSONParams(*jsonParams)

			params = []byte(jsonParams)
		}

		err = deployer.CreateContract(compiledContract, params, target.name, *force, *aslib, *gasLimit)
		if err != nil {
			fmt.Println("\u2757\ufe0f \033[36mdeploy\033[0m", err)
			return
		}

		// Add related contracts to repo
		relatedContracts, err := compiler.RelatedContracts()
		if err != nil {
			return err
		}

		if len(relatedContracts) > 0 {
			for name, c := range relatedContracts {
				repo.Related[name] = c
			}

			err = repo.Commit()
			if err != nil {
				return
			}
		}

		newContracts := repo.UnconfirmedContracts()
		if *noconfirm == false && len(newContracts) != 0 {
			// Force local chain to generate a block immediately.
			allowFastConfirm := *solarEnv == "development" || *solarEnv == "test"
			if *noFastConfirm == false && allowFastConfirm {
				//fmt.Println("call deployer.Mine")
				err = deployer.Mine()
				if err != nil {
					log.Println(err)
				}
			}

			err := repo.ConfirmAll(getConfirmUpdateProgressFunc(), deployer.ConfirmContract)
			if err != nil {
				return err
			}

			var deployedContract *contract.DeployedContract
			if *aslib {
				deployedContract, _ = repo.GetLib(target.name)
			} else {
				deployedContract, _ = repo.Get(target.name)
			}

			if deployedContract == nil {
				return errors.New("failed to deploy contract")
			}

			fmt.Printf("   \033[36mdeployed\033[0m %s => %s\n", target.name, deployedContract.Address)
		}

		return
	}
}

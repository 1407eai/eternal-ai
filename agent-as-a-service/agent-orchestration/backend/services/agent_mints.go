package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/daos"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/errs"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/helpers"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/models"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/serializers"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/binds/systempromptmanager"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/lighthouse"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/types/numeric"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
)

func (s *Service) JobAgentMintNft(ctx context.Context) error {
	err := s.JobRunCheck(
		ctx, "JobAgentMintNft",
		func() error {
			agents, err := s.dao.FindAgentInfo(
				daos.GetDBMainCtx(ctx),
				map[string][]interface{}{
					"agent_id != ''":        {},
					"agent_contract_id = ?": {""},
					"agent_nft_minted = ?":  {false},
					`twin_twitter_usernames is null 
						or twin_twitter_usernames = '' 
						or (twin_twitter_usernames != '' and twin_status = ?)
					`: {models.TwinStatusDoneSuccess},
					"scan_enabled = ?": {true},
					`(1 != 1
						or (network_id = ? and eai_balance >= 5999.9)
						or (network_id = ? and eai_balance >= 299.9)
						or (network_id = ? and eai_balance >= 174.9)
						or (network_id not in (?) and eai_balance >= 9.99)
						or (ref_tweet_id > 0 and eai_balance >= 0)
					)
					`: {
						models.SHARDAI_CHAIN_ID,
						models.ETHEREUM_CHAIN_ID,
						models.SOLANA_CHAIN_ID,
						[]uint64{
							models.SHARDAI_CHAIN_ID,
							models.ETHEREUM_CHAIN_ID,
							models.SOLANA_CHAIN_ID,
						},
					},
				},
				map[string][]interface{}{},
				[]string{
					"updated_at asc",
				},
				0,
				10,
			)
			if err != nil {
				return errs.NewError(err)
			}
			var retErr error
			for _, agent := range agents {
				err = s.AgentMintNft(ctx, agent.ID)
				if err != nil {
					retErr = errs.MergeError(retErr, errs.NewError(err))
				}
				err = s.AgentCreateMissionDefault(ctx, agent.ID)
				if err != nil {
					retErr = errs.MergeError(retErr, errs.NewError(err))
				}
				time.Sleep(10 * time.Second)
			}
			return retErr
		},
	)
	if err != nil {
		return errs.NewError(err)
	}
	return nil
}

func (s *Service) AgentMintNft(ctx context.Context, agentInfoID uint) error {
	err := s.JobRunCheck(
		ctx,
		fmt.Sprintf("AgentMintNft_%d", agentInfoID),
		func() error {
			agent, err := s.dao.FirstAgentInfoByID(
				daos.GetDBMainCtx(ctx),
				agentInfoID,
				map[string][]interface{}{},
				false,
			)
			if err != nil {
				return errs.NewError(err)
			}
			if agent.AgentContractID == "" &&
				!agent.AgentNftMinted {
				var isOk bool
				var mintFee, checkFee *big.Float
				switch agent.NetworkID {
				case models.SHARDAI_CHAIN_ID:
					{
						checkFee = numeric.NewFloatFromString("5999.9")
						mintFee = numeric.NewFloatFromString("200")
					}
				case models.ETHEREUM_CHAIN_ID:
					{
						checkFee = numeric.NewFloatFromString("299.9")
						mintFee = numeric.NewFloatFromString("270.0")
					}
				case models.SOLANA_CHAIN_ID:
					{
						checkFee = numeric.NewFloatFromString("174.9")
						mintFee = numeric.NewFloatFromString("125.0")
					}
				case models.BITTENSOR_CHAIN_ID:
					{
						checkFee = numeric.NewFloatFromString("569.9")
						mintFee = numeric.NewFloatFromString("20.0")
					}
				default:
					{
						checkFee = numeric.NewFloatFromString("0.99")
						mintFee = numeric.NewFloatFromString("0.0")
					}
				}
				if agent.RefTweetID > 0 {
					mintFee = numeric.NewFloatFromString("0.0")
				}
				if agent.EaiBalance.Float.Cmp(checkFee) >= 0 {
					isOk = true
				}
				if isOk {
					updateAgentFields := map[string]interface{}{
						"agent_nft_minted": true,
					}
					if agent.TokenMode == string(models.TokenSetupEnumAutoCreate) && agent.TokenAddress == "" && agent.TokenStatus == "" {
						updateAgentFields["token_status"] = "pending"
					}
					err = daos.GetDBMainCtx(ctx).Model(agent).Updates(updateAgentFields).Error
					if err != nil {
						return errs.NewError(err)
					}
					for i := 0; i < 5; i++ {
						err = s.MintAgent(ctx, agent.ID)
						if err == nil {
							break
						}
					}
					if err != nil {
						_ = daos.GetDBMainCtx(ctx).
							Model(agent).
							Updates(
								map[string]interface{}{
									"scan_error": "mint nft error " + err.Error(),
								},
							).
							Error
						return errs.NewError(err)
					} else {
						err = daos.GetDBMainCtx(ctx).
							Model(agent).
							Updates(
								map[string]interface{}{
									"eai_balance": gorm.Expr("eai_balance - ?", numeric.NewBigFloatFromFloat(mintFee)),
									"mint_fee":    numeric.NewBigFloatFromFloat(mintFee),
								},
							).
							Error
						if err != nil {
							return errs.NewError(err)
						}
						if mintFee.Cmp(big.NewFloat(0)) > 0 {
							_ = s.dao.Create(
								daos.GetDBMainCtx(ctx),
								&models.AgentEaiTopup{
									NetworkID:      agent.NetworkID,
									EventId:        fmt.Sprintf("agent_mint_fee_%d", agent.ID),
									AgentInfoID:    agent.ID,
									Type:           models.AgentEaiTopupTypeSpent,
									Amount:         numeric.NewBigFloatFromFloat(mintFee),
									Status:         models.AgentEaiTopupStatusDone,
									DepositAddress: agent.ETHAddress,
									ToAddress:      agent.ETHAddress,
									Toolset:        "mint_fee",
								},
							)
						}
					}
				}
			}
			return nil
		},
	)
	if err != nil {
		return errs.NewError(err)
	}
	return nil
}

func (s *Service) JobRetryAgentMintNft(ctx context.Context) error {
	err := s.JobRunCheck(
		ctx, "JobAgentMintNft",
		func() error {
			agents, err := s.dao.FindAgentInfo(
				daos.GetDBMainCtx(ctx),
				map[string][]interface{}{
					"agent_contract_id = ?": {""},
					"agent_nft_minted = ?":  {true},
					"mint_hash != ?":        {""},
					"network_id in (?)": {
						[]uint64{
							models.BASE_CHAIN_ID,
							models.HERMES_CHAIN_ID,
							models.ARBITRUM_CHAIN_ID,
							models.ZKSYNC_CHAIN_ID,
							models.POLYGON_CHAIN_ID,
							models.BSC_CHAIN_ID,
							models.APE_CHAIN_ID,
							models.AVALANCHE_C_CHAIN_ID,
							models.ABSTRACT_TESTNET_CHAIN_ID,
							models.DUCK_CHAIN_ID,
						},
					},
				},
				map[string][]interface{}{},
				[]string{
					"rand()",
				},
				0,
				1000,
			)
			if err != nil {
				return errs.NewError(err)
			}
			var retErr error
			for _, agent := range agents {
				fmt.Println(agent.MintHash)
				err = s.GetEVMClient(ctx, agent.NetworkID).TransactionConfirmed(agent.MintHash)
				if err != nil {
					fmt.Println(err.Error())
					if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "transaction is not Successful") {
						err = daos.GetDBMainCtx(ctx).
							Model(agent).
							Updates(
								map[string]interface{}{
									"eai_balance":      gorm.Expr("eai_balance - ?", agent.MintFee),
									"agent_nft_minted": false,
									"mint_hash":        "",
								},
							).
							Error
						if err != nil {
							return errs.NewError(err)
						}
					}
				} else {
					s.MemeEventsByTransaction(ctx, agent.NetworkID, agent.MintHash)
				}
			}
			return retErr
		},
	)
	if err != nil {
		return errs.NewError(err)
	}
	return nil
}

func (s *Service) JobRetryAgentMintNftError(ctx context.Context) error {
	err := s.JobRunCheck(
		ctx, "JobAgentMintNft",
		func() error {
			agents, err := s.dao.FindAgentInfo(
				daos.GetDBMainCtx(ctx),
				map[string][]interface{}{
					"agent_contract_id = ?": {""},
					"agent_nft_minted = ?":  {true},
					"mint_hash = ?":         {""},
					"scan_error != ?":       {""},
					"network_id in (?)": {
						[]uint64{
							models.BASE_CHAIN_ID,
							models.HERMES_CHAIN_ID,
							models.ARBITRUM_CHAIN_ID,
							models.ZKSYNC_CHAIN_ID,
							models.POLYGON_CHAIN_ID,
							models.BSC_CHAIN_ID,
							models.APE_CHAIN_ID,
							models.AVALANCHE_C_CHAIN_ID,
							models.ABSTRACT_TESTNET_CHAIN_ID,
							models.DUCK_CHAIN_ID,
						},
					},
				},
				map[string][]interface{}{},
				[]string{
					"rand()",
				},
				0,
				1000,
			)
			if err != nil {
				return errs.NewError(err)
			}
			var retErr error
			for _, agent := range agents {
				if strings.Contains(agent.ScanError, "mint nft error") {
					err = daos.GetDBMainCtx(ctx).
						Model(agent).
						Updates(
							map[string]interface{}{
								"eai_balance":      gorm.Expr("eai_balance - ?", agent.MintFee),
								"agent_nft_minted": false,
								"scan_error":       "",
							},
						).
						Error
					if err != nil {
						return errs.NewError(err)
					}
				}
			}
			return retErr
		},
	)
	if err != nil {
		return errs.NewError(err)
	}
	return nil
}

func (s *Service) MintAgent(ctx context.Context, agentInfoID uint) error {
	agentInfo, err := s.dao.FirstAgentInfoByID(
		daos.GetDBMainCtx(ctx),
		agentInfoID,
		map[string][]interface{}{},
		false,
	)
	if err != nil {
		return errs.NewError(err)
	}
	if agentInfo != nil {
		if agentInfo.MintHash == "" {
			switch agentInfo.NetworkID {
			case models.SHARDAI_CHAIN_ID,
				models.HERMES_CHAIN_ID,
				models.BASE_CHAIN_ID,
				models.ETHEREUM_CHAIN_ID,
				models.ARBITRUM_CHAIN_ID,
				models.BSC_CHAIN_ID,
				models.POLYGON_CHAIN_ID,
				models.ZKSYNC_CHAIN_ID,
				models.APE_CHAIN_ID,
				models.AVALANCHE_C_CHAIN_ID,
				models.ABSTRACT_TESTNET_CHAIN_ID,
				models.BITTENSOR_CHAIN_ID,
				models.DUCK_CHAIN_ID:
				{
					agentUriData := models.AgentUriData{
						Name: agentInfo.AgentName,
					}
					agentUriBytes, err := json.Marshal(agentUriData)
					if err != nil {
						return errs.NewError(err)
					}
					uriHash, err := s.IpfsUploadDataForName(ctx, fmt.Sprintf("%v_%v", agentInfo.AgentID, "uri"), agentUriBytes)
					if err != nil {
						return errs.NewError(err)
					}
					systemContentHash, err := s.IpfsUploadDataForName(ctx, fmt.Sprintf("%v_%v", agentInfo.AgentID, "system_content"), []byte(agentInfo.SystemPrompt))
					if err != nil {
						return errs.NewError(err)
					}
					txHash, err := s.GetEVMClient(ctx, agentInfo.NetworkID).SystemPromptManagerMint(
						s.conf.GetConfigKeyString(agentInfo.NetworkID, "agent_contract_address"),
						s.GetAddressPrk(
							helpers.RandomInStrings(
								strings.Split(s.conf.GetConfigKeyString(agentInfo.NetworkID, "agent_admin_address"), ","),
							),
						),
						common.HexToAddress(agentInfo.Creator),
						"ipfs://"+uriHash,
						[]byte("ipfs://"+systemContentHash),
						models.ConvertBigFloatToWei(&agentInfo.InferFee.Float, 18),
					)
					if err != nil {
						return errs.NewError(err)
					}
					err = daos.GetDBMainCtx(ctx).
						Model(agentInfo).
						Updates(
							map[string]interface{}{
								"agent_contract_address": s.conf.GetConfigKeyString(agentInfo.NetworkID, "agent_contract_address"),
								"mint_hash":              txHash,
								"status":                 models.AssistantStatusMinting,
								"enabled":                true,
								"reply_enabled":          true,
							},
						).Error
					if err != nil {
						return errs.NewError(err)
					}
				}
			case models.SOLANA_CHAIN_ID:
				{
					err = daos.GetDBMainCtx(ctx).
						Model(agentInfo).
						Updates(
							map[string]interface{}{
								"agent_contract_address": s.conf.GetConfigKeyString(agentInfo.NetworkID, "agent_contract_address"),
								"agent_contract_id":      strconv.FormatUint(uint64(agentInfo.ID), 10),
								"status":                 models.AssistantStatusReady,
								"enabled":                true,
								"reply_enabled":          true,
							},
						).Error
					if err != nil {
						return errs.NewError(err)
					}
				}
			default:
				{
					return errs.NewError(errs.ErrBadRequest)
				}
			}
		}
	}
	return nil
}

func (s *Service) SystemPromptManagerNewTokenEvent(ctx context.Context, networkID uint64, event *systempromptmanager.SystemPromptManagerNewToken) error {
	agentInfo, err := s.dao.FirstAgentInfo(
		daos.GetDBMainCtx(ctx),
		map[string][]interface{}{
			"network_id = ?": {networkID},
			"mint_hash = ?":  {event.Raw.TxHash.Hex()},
		},
		map[string][]interface{}{},
		[]string{},
	)
	if err != nil {
		return errs.NewError(err)
	}
	if agentInfo != nil {
		data, _, err := lighthouse.DownloadDataSimple(event.Uri)
		if err != nil {
			dataInfo := map[string]interface{}{}
			err = json.Unmarshal([]byte(event.Uri), &dataInfo)
			if err != nil {
				return errs.NewError(err)
			}
			uri := dataInfo["agent_uri"].(string)
			data, _, err = lighthouse.DownloadDataSimple(uri)
			if err != nil {
				return errs.NewError(err)
			}
		}
		systemPrompt, _, err := lighthouse.DownloadDataSimple(string(event.SysPrompt))
		if err != nil {
			return errs.NewError(err)
		}
		var info models.AgentUriData
		err = json.Unmarshal(data, &info)
		if err != nil {
			return errs.NewError(err)
		}
		err = daos.GetDBMainCtx(ctx).
			Model(agentInfo).
			Updates(
				map[string]interface{}{
					"agent_name":        info.Name,
					"creator":           strings.ToLower(event.Minter.Hex()),
					"agent_contract_id": event.TokenId.String(),
					"status":            models.AssistantStatusReady,
					"system_prompt":     string(systemPrompt),
					"enabled":           true,
					"reply_enabled":     true,
				},
			).Error
		if err != nil {
			return errs.NewError(err)
		}
	}
	return nil
}

func (s *Service) SystemPromptManagerAgentDataUpdateEvent(ctx context.Context, networkID uint64, event *systempromptmanager.SystemPromptManagerAgentDataUpdate) error {
	contractAgentID := event.AgentId.String()
	lightHouseHash := string(event.NewSysPrompt)
	systemPromptBytes, _, err := lighthouse.DownloadDataSimple(lightHouseHash)
	if err != nil {
		if !strings.HasPrefix(lightHouseHash, "ipfs://") {
			systemPromptBytes = event.NewSysPrompt
		} else {
			return errs.NewError(err)
		}
	}
	err = daos.GetDBMainCtx(ctx).
		Model(&models.AgentInfo{}).
		Where("network_id = ?", networkID).
		Where("agent_contract_address = ?", strings.ToLower(event.Raw.Address.Hex())).
		Where("agent_contract_id = ?", contractAgentID).
		Updates(
			map[string]interface{}{
				"system_prompt": string(systemPromptBytes),
			},
		).Error
	if err != nil {
		return errs.NewError(err)
	}
	return nil
}

func (s *Service) SystemPromptManagerAgentURIUpdateEvent(ctx context.Context, networkID uint64, event *systempromptmanager.SystemPromptManagerAgentURIUpdate) error {
	contractAgentID := event.AgentId.String()
	uri := event.Uri
	dataBytes, _, err := lighthouse.DownloadDataSimple(uri)
	if err != nil {
		return err
	}
	agentUriData := models.AgentUriData{}
	err = json.Unmarshal(dataBytes, &agentUriData)
	if err != nil {
		return err
	}
	err = daos.GetDBMainCtx(ctx).
		Model(&models.AgentInfo{}).
		Where("network_id = ?", networkID).
		Where("agent_contract_address = ?", strings.ToLower(event.Raw.Address.Hex())).
		Where("agent_contract_id = ?", contractAgentID).
		Updates(
			map[string]interface{}{
				"agent_name": agentUriData.Name,
				"uri":        uri,
			},
		).Error
	if err != nil {
		return errs.NewError(err)
	}
	return nil
}

func (s *Service) ExecuteUpdateAgentInfoInContract(ctx context.Context, assistant *models.AgentInfo, request *serializers.UpdateAgentAssistantInContractRequest) (*serializers.UpdateAgentAssistantInContractResponse, error) {
	var txUpdateNameHash, txUpdateSystemPromptHash string
	switch assistant.NetworkID {
	case models.BASE_CHAIN_ID,
		models.ETHEREUM_CHAIN_ID,
		models.ARBITRUM_CHAIN_ID,
		models.BSC_CHAIN_ID,
		models.POLYGON_CHAIN_ID,
		models.ZKSYNC_CHAIN_ID:
		{
			ethClient := s.GetEVMClient(ctx, assistant.NetworkID)
			instanceABI, err := abi.JSON(strings.NewReader(systempromptmanager.SystemPromptManagerMetaData.ABI))
			if err != nil {
				return nil, errs.NewError(err)
			}
			agentId, ok := new(big.Int).SetString(assistant.AgentContractID, 10)
			if !ok {
				return nil, errs.NewError(fmt.Errorf("error while getting agent id :%v", assistant.AgentContractID))
			}
			{
				randomNonceName, ok := new(big.Int).SetString(request.RandomNonceName, 10)
				if !ok {
					return nil, fmt.Errorf("error while getting  random nonce name:%v", request.RandomNonceName)
				}
				input, err := instanceABI.Pack("updateAgentUriWithSignature", agentId, request.HashName, randomNonceName, common.Hex2Bytes(request.SignatureName[2:]))
				if err != nil {
					return nil, errs.NewError(err)
				}
				txUpdateNameHash, err = ethClient.Transact(
					assistant.AgentContractAddress,
					s.GetAddressPrk(
						helpers.RandomInStrings(
							strings.Split(s.conf.GetConfigKeyString(assistant.NetworkID, "agent_admin_address"), ","),
						),
					),
					input,
					common.Big0,
				)
				if err != nil {
					return nil, errs.NewError(err)
				}
			}
			{
				randomNonceSystemPrompt, ok := new(big.Int).SetString(request.RandomNonceSystemPrompt, 10)
				if !ok {
					return nil, fmt.Errorf("error while getting random nonce system prompt :%v", request.RandomNonceSystemPrompt)
				}
				input, err := instanceABI.Pack("updateAgentDataWithSignature", agentId, []byte(request.HashSystemPrompt),
					big.NewInt(0), randomNonceSystemPrompt, common.Hex2Bytes(request.SignatureSystemPrompt[2:]))
				if err != nil {
					return nil, errs.NewError(err)
				}
				txUpdateSystemPromptHash, err = ethClient.Transact(
					assistant.AgentContractAddress,
					s.GetAddressPrk(
						helpers.RandomInStrings(
							strings.Split(s.conf.GetConfigKeyString(assistant.NetworkID, "agent_admin_address"), ","),
						),
					),
					input,
					common.Big0,
				)
				if err != nil {
					return nil, errs.NewError(err)
				}
			}
		}
	default:
		{
			txUpdateNameHash = uuid.NewString()
			txUpdateSystemPromptHash = uuid.NewString()
		}
	}
	return &serializers.UpdateAgentAssistantInContractResponse{
		TxUpdateSystemPrompt: txUpdateSystemPromptHash,
		TxUpdateName:         txUpdateNameHash,
	}, nil
}

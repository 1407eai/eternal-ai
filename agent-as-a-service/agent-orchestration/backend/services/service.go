package services

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/configs"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/daos"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/errs"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/aidojo"
	blockchainutils "github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/blockchain_utils"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/bridgeapi"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/btcapi"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/coingecko"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/coinmarketcap"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/core"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/dexscreener"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/ethapi"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/googlestorage"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/openai"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/opensea"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/pumfun"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/rapid"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/taapi"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/twitter"
	"github.com/eternalai-org/eternal-ai/agent-as-a-service/agent-orchestration/backend/services/3rd/zkapi"
	"github.com/go-redis/redis"
	"github.com/jinzhu/gorm"
)

type Service struct {
	jobRunMap   map[string]bool
	jobMutex    sync.Mutex
	jobDisabled bool
	// config
	conf *configs.Config
	// clients
	rdb             *redis.Client
	coreClient      *core.Client
	gsClient        *googlestorage.Client
	openais         map[string]*openai.OpenAI
	ethApiMap       map[uint64]*ethapi.Client
	zkApiMap        map[uint64]*zkapi.Client
	rapid           *rapid.Rapid
	blockchainUtils *blockchainutils.Client
	btcAPI          *btcapi.Client
	pumfunAPI       *pumfun.Client
	cmc             *coinmarketcap.CoinMarketCap
	cgc             *coingecko.CoinGeckoAPI
	twitterAPI      *twitter.Client
	twitterWrapAPI  *twitter.Client
	dojoAPI         *aidojo.AiDojoBackend
	bridgeAPI       *bridgeapi.BridgeApi
	dexscreener     *dexscreener.DexScreenerAPI
	openseaService  *opensea.OpenseaService
	taapi           *taapi.TaApi
	// daos
	dao *daos.DAO
}

func NewService(conf *configs.Config) *Service {
	s := &Service{
		jobRunMap: map[string]bool{},
		jobMutex:  sync.Mutex{},
		//
		conf: conf,
		//
		rdb: redis.NewClient(&redis.Options{
			Addr:     conf.Redis.Addr,
			Password: conf.Redis.Password,
			DB:       conf.Redis.Db,
		}),
		coreClient: &core.Client{
			BaseURL: conf.Core.Url,
		},
		gsClient: googlestorage.InitClient(conf.GsStorage.CredentialsFile, conf.GsStorage.BucketName),
		openais: map[string]*openai.OpenAI{
			"Agent": openai.NewAgentAI(conf.Ai.ApiKey, conf.AiImageApiKey),
			"Lama": openai.NewOpenAI(conf.Ai.ChatUrl, conf.Ai.ReadImageUrl,
				conf.Ai.ApiKey, "NousResearch/Hermes-3-Llama-3.1-70B-FP8",
				"You are a helpful assistant"),
		},
		ethApiMap: map[uint64]*ethapi.Client{},
		zkApiMap:  map[uint64]*zkapi.Client{},
		rapid:     rapid.NewRapid(conf.RapidApiKey),
		blockchainUtils: &blockchainutils.Client{
			BaseURL: conf.BlockchainUtils.Url,
		},
		btcAPI: &btcapi.Client{
			Chain:             "btc",
			Network:           conf.Btc.Network,
			Token:             conf.Btc.BcyToken,
			QNUrl:             conf.Btc.QnUrl,
			SdkUrl:            "",
			BlockstreamUrl:    "https://blockstream.info",
			MempoolUrl:        "https://mempool.space",
			HirosoUrl:         "https://api.hiro.so",
			BlockchainInfoUrl: "https://blockchain.info",
		},
		pumfunAPI: &pumfun.Client{
			BaseUrl: "https://frontend-api.pump.fun",
		},
		cmc: coinmarketcap.NewCoinMarketCap(conf.CMCApiKey),
		cgc: coingecko.NewCoinGeckoAPI(),
		// daos
		dao: &daos.DAO{},
		twitterAPI: twitter.NewClient(conf.Twitter.Token, conf.Twitter.ConsumerKey, conf.Twitter.ConsumerSecret,
			conf.Twitter.AccessToken, conf.Twitter.AccessSecret,
			conf.Twitter.OauthClientId, conf.Twitter.OauthClientSecret,
			conf.Twitter.RedirectUri,
		),
		twitterWrapAPI: twitter.NewTwitterWrapClient(conf.Twitter.TokenForTwitterData),
		dojoAPI:        aidojo.NewAiDojoBackend(conf.AiDojoBackend.Url, conf.AiDojoBackend.ApiKey),
		bridgeAPI:      bridgeapi.NewBridgeApi(conf.EternalaiBridgesUrl),
		dexscreener:    dexscreener.NewDexScreenerAPI(),
		openseaService: opensea.NewOpensea(conf.OpenseaAPIKey),
		taapi:          taapi.NewTaApi(conf.TaApiKey),
	}
	return s
}

func (s *Service) GetAddressPrk(address string) string {
	prkHex, err := s.coreClient.GetAddressPrk(
		address,
	)
	if err != nil {
		panic(err)
	}
	return prkHex
}

func (s *Service) JobRunCheck(ctx context.Context, jobId string, jobFunc func() error) error {
	s.jobMutex.Lock()
	isRun := s.jobRunMap[jobId]
	jobDisabled := s.jobDisabled
	s.jobMutex.Unlock()
	if !isRun && !jobDisabled {
		s.jobMutex.Lock()
		s.jobRunMap[jobId] = true
		s.jobMutex.Unlock()
		defer func() {
			s.jobMutex.Lock()
			s.jobRunMap[jobId] = false
			s.jobMutex.Unlock()
			if rval := recover(); rval != nil {
				err := errs.NewError(errors.New(fmt.Sprint(rval)))
				stacktrace := err.(*errs.Error).Stacktrace()
				fmt.Println(time.Now(), jobId, "panic", err.Error(), stacktrace)
			}
		}()
		fmt.Println(time.Now(), jobId, "begin")
		err := jobFunc()
		if err != nil {
			err = errs.NewError(err)
			stacktrace := err.(*errs.Error).Stacktrace()
			fmt.Println(time.Now(), jobId, "error", err.Error(), stacktrace)
		} else {
			fmt.Println(time.Now(), jobId, "end")
		}
	}
	return nil
}

func (s *Service) JobRun(ctx context.Context, jobName string, duration time.Duration, jobFunc func() error) {
	s.jobMutex.Lock()
	isRun := s.jobRunMap[jobName]
	s.jobMutex.Unlock()
	if !isRun {
		s.jobMutex.Lock()
		s.jobRunMap[jobName] = true
		s.jobMutex.Unlock()
		go func() {
			for {
				fmt.Println(time.Now(), jobName, "begin")
				err := func() error {
					defer func() {
						if rval := recover(); rval != nil {
							err := errs.NewError(errors.New(fmt.Sprint(rval)))
							stacktrace := err.(*errs.Error).Stacktrace()
							fmt.Println(time.Now(), jobName, "panic", err.Error(), stacktrace)
						}
					}()
					err := jobFunc()
					if err != nil {
						return errs.NewError(err)
					}
					return nil
				}()
				if err != nil {
					err = errs.NewError(err)
					stacktrace := err.(*errs.Error).Stacktrace()
					fmt.Println(time.Now(), jobName, "error", err.Error(), stacktrace)
				} else {
					fmt.Println(time.Now(), jobName, "end")
				}
				time.Sleep(duration)
			}
		}()
	}
}

func (s *Service) VerifyAddressSignature(ctx context.Context, networkID uint64, address string, message string, signature string) error {
	err := s.GetEthereumClient(ctx, networkID).ValidateMessageSignature(
		message,
		signature,
		address,
	)
	if err != nil {
		return errs.NewError(err)
	}
	return nil
}

func (s *Service) GetTokenMarketPrice(tx *gorm.DB, symbol string) *big.Float {
	cachedKey := fmt.Sprintf(`GetTokenMarketPrice_%s`, symbol)
	tokenPrice := big.NewFloat(0)
	_ = s.GetRedisCachedWithKey(cachedKey, &tokenPrice)
	if tokenPrice.Cmp(big.NewFloat(0)) <= 0 {
		tkPrice, _, err := s.dao.GetTokenMarketPrice(tx, symbol)
		if err != nil {
			return big.NewFloat(0)
		}
		tokenPrice = &tkPrice.Float
		_ = s.SetRedisCachedWithKey(cachedKey, tokenPrice, 5*time.Minute)
	}
	return tokenPrice
}

func (s *Service) GetMapTokenPrice(ctx context.Context) map[string]*big.Float {
	cachedKey := `AgentGetMapTokenPrice`
	mapTokenPrice := map[string]*big.Float{}
	err := s.GetRedisCachedWithKey(cachedKey, &mapTokenPrice)
	if err != nil {
		mapTokenPrice["BTC"] = s.GetTokenMarketPrice(daos.GetDBMainCtx(ctx), "BTC")
		mapTokenPrice["ETH"] = s.GetTokenMarketPrice(daos.GetDBMainCtx(ctx), "ETH")
		mapTokenPrice["BVM"] = s.GetTokenMarketPrice(daos.GetDBMainCtx(ctx), "BVM")
		mapTokenPrice["EAI"] = s.GetTokenMarketPrice(daos.GetDBMainCtx(ctx), "EAI")
		mapTokenPrice["SOL"] = s.GetTokenMarketPrice(daos.GetDBMainCtx(ctx), "SOL")
		s.SetRedisCachedWithKey(cachedKey, mapTokenPrice, 1*time.Minute)
	}
	return mapTokenPrice
}
func (s *Service) GetDao() *daos.DAO {
	return s.dao
}